import { cp, mkdir, readdir, rm, writeFile } from "node:fs/promises";
import { join, relative } from "node:path";

const target = process.argv[2];
if (!target) {
  console.error("usage: bun platform.ts <path to emusoi/platform>");
  process.exit(64);
}

const dist = join(import.meta.dir, "dist");
const app = join(target, "apps", "mia");

async function walk(dir: string): Promise<string[]> {
  const out: string[] = [];
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) out.push(...(await walk(path)));
    else out.push(path);
  }
  return out;
}

const files = (await walk(dist)).map((path) => relative(dist, path)).sort();
const key = (path: string) => path.replace(/\//g, "--");

await rm(join(app, "site"), { recursive: true, force: true });
await mkdir(app, { recursive: true });
await cp(dist, join(app, "site"), { recursive: true });

await writeFile(
  join(app, "kustomization.yaml"),
  `# Written by mia-core/site/platform.ts from the built site; run \`make site\` there, then this.
resources: [mia.yaml]
configMapGenerator:
  - name: mia-site
    files:
${files.map((f) => `      - ${key(f)}=site/${f}`).join("\n")}
  - name: mia-nginx
    files: [default.conf]
`,
);

await writeFile(
  join(app, "default.conf"),
  `server {
  listen 8080;
  root /usr/share/nginx/html;
  charset utf-8;
  absolute_redirect off;
  location / {
    try_files $uri $uri.html $uri/index.html =404;
    add_header Cache-Control "no-cache";
  }
  location ~* \\.(css|js|json|svg)$ {
    add_header Cache-Control "public, max-age=3600";
  }
  error_page 404 /404.html;
}
`,
);

await writeFile(
  join(app, "mia.yaml"),
  `apiVersion: apps/v1
kind: Deployment
metadata:
  name: mia
spec:
  replicas: 1
  selector:
    matchLabels: { app: mia }
  template:
    metadata:
      labels: { app: mia }
    spec:
      containers:
        - name: nginx
          image: nginx:1.27-alpine
          ports: [{ containerPort: 8080 }]
          readinessProbe:
            httpGet: { path: /, port: 8080 }
          resources:
            requests: { cpu: 5m, memory: 16Mi }
            limits: { memory: 64Mi }
          volumeMounts:
            - { name: site, mountPath: /usr/share/nginx/html, readOnly: true }
            - { name: nginx, mountPath: /etc/nginx/conf.d, readOnly: true }
      volumes:
        - name: site
          configMap:
            name: mia-site
            items:
${files.map((f) => `              - { key: ${key(f)}, path: ${f} }`).join("\n")}
        - name: nginx
          configMap:
            name: mia-nginx
---
apiVersion: v1
kind: Service
metadata:
  name: mia
spec:
  selector: { app: mia }
  ports: [{ port: 80, targetPort: 8080 }]
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: mia
spec:
  ingressClassName: traefik
  rules:
    - host: mia.emusoi.app
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service: { name: mia, port: { number: 80 } }
`,
);

console.log(`wrote ${relative(process.cwd(), app) || app}: ${files.length} files for mia.emusoi.app`);
