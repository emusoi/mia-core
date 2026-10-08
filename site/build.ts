import { marked, type Tokens } from "marked";
import { createHighlighter } from "shiki";
import { mkdir, readFile, rm, writeFile, copyFile } from "node:fs/promises";
import { join } from "node:path";

const root = join(import.meta.dir, "..");
const docs = join(root, "docs");
const out = join(import.meta.dir, "dist");
const repo = "https://github.com/emusoi/mia-core";
const version = Date.now().toString(36);

const pages = [
  { file: "why.md", slug: "why", group: "Start" },
  { file: "getting-started.md", slug: "getting-started", group: "Start" },
  { file: "worktrees.md", slug: "worktrees", group: "Using mia" },
  { file: "stacks.md", slug: "stacks", group: "Using mia" },
  { file: "environments.md", slug: "environments", group: "Using mia" },
  { file: "dashboard.md", slug: "dashboard", group: "Using mia" },
  { file: "configuration.md", slug: "configuration", group: "Reference" },
  { file: "commands.md", slug: "commands", group: "Reference" },
  { file: "plugins.md", slug: "plugins", group: "Reference" },
];

const highlighter = await createHighlighter({
  themes: ["min-light", "min-dark"],
  langs: ["bash", "toml", "json", "go", "lua"],
});

function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/<[^>]+>/g, "")
    .replace(/[`*_]/g, "")
    .replace(/[^a-z0-9 -]/g, "")
    .trim()
    .replace(/\s+/g, "-");
}

function escape(text: string): string {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

function href(target: string): string {
  if (/^[a-z]+:/.test(target) || target.startsWith("#")) return target;
  const [path, hash] = target.split("#");
  const anchor = hash ? `#${hash}` : "";
  if (path === "" ) return anchor;
  if (path === "README.md") return `/docs/${anchor}`;
  const page = pages.find((p) => p.file === path);
  if (page) return `/docs/${page.slug}${anchor}`;
  return `${repo}/blob/main/${path.replace(/^\.\.\//, "")}${anchor}`;
}

function code(text: string, lang: string | undefined): string {
  const known = lang && highlighter.getLoadedLanguages().includes(lang);
  if (!known) {
    return `<div class="code"><pre class="plain"><code>${escape(text)}</code></pre><button class="copy" type="button" aria-label="Copy">Copy</button></div>`;
  }
  const html = highlighter.codeToHtml(text, {
    lang: known ? lang! : "text",
    themes: { light: "min-light", dark: "min-dark" },
    defaultColor: false,
  });
  return `<div class="code">${html}<button class="copy" type="button" aria-label="Copy">Copy</button></div>`;
}

type Heading = { depth: number; text: string; id: string };

function render(markdown: string): { html: string; title: string; headings: Heading[]; lead: string } {
  const headings: Heading[] = [];
  let title = "";
  let lead = "";
  const seen = new Map<string, number>();
  const renderer = new marked.Renderer();
  renderer.heading = function ({ tokens, depth, text }: Tokens.Heading) {
    const inner = this.parser.parseInline(tokens);
    if (depth === 1) {
      title = text.replace(/`/g, "");
      return `<h1>${inner}</h1>`;
    }
    let id = slugify(text);
    const n = seen.get(id) ?? 0;
    seen.set(id, n + 1);
    if (n) id = `${id}-${n}`;
    headings.push({ depth, text: text.replace(/`/g, ""), id });
    return `<h${depth} id="${id}"><a class="anchor" href="#${id}" aria-hidden="true">#</a>${inner}</h${depth}>`;
  };
  renderer.code = ({ text, lang }: Tokens.Code) => code(text, lang || undefined);
  renderer.link = function ({ href: target, tokens, title: hint }: Tokens.Link) {
    const inner = this.parser.parseInline(tokens);
    const to = href(target);
    const external = /^https?:/.test(to) && !to.startsWith(repo) ? ' rel="noopener"' : "";
    return `<a href="${escape(to)}"${hint ? ` title="${escape(hint)}"` : ""}${external}>${inner}</a>`;
  };
  renderer.table = function (token: Tokens.Table) {
    const head = token.header.map((cell) => `<th>${this.parser.parseInline(cell.tokens)}</th>`).join("");
    const body = token.rows
      .map((row) => `<tr>${row.map((cell) => `<td>${this.parser.parseInline(cell.tokens)}</td>`).join("")}</tr>`)
      .join("");
    return `<div class="table"><table><thead><tr>${head}</tr></thead><tbody>${body}</tbody></table></div>`;
  };
  renderer.paragraph = function ({ tokens, text }: Tokens.Paragraph) {
    if (!lead) lead = text.replace(/[`*_\[\]]/g, "").replace(/\(([^)]*)\)/g, "").slice(0, 180);
    return `<p>${this.parser.parseInline(tokens)}</p>`;
  };
  const html = marked.parse(markdown, { renderer, gfm: true, async: false }) as string;
  return { html, title, headings, lead };
}

const lit = [3, 14, 27, 31, 46, 52, 68, 75, 81, 99];
const movers = new Map<number, number>([[3,-8.59],[7,-1.42],[11,-2.14],[14,-8.14],[16,-4.16],[17,-6.7],[20,-2.53],[24,-11.47],[27,-9.51],[29,-5.69],[31,-8.04],[33,-0.86],[37,-0.07],[46,-8.91],[47,-3.94],[49,-2.31],[52,-10.57],[56,-1.65],[61,-5.38],[64,-6.28],[68,-7.91],[71,-6.65],[75,-11.04],[76,-4.99],[81,-10.32],[91,-6.09],[93,-3.48],[99,-9.64]]);

const mark = (size: number, moving = false) => {
  const cells: string[] = [];
  for (let i = 0; i < 100; i++) {
    const x = 76 + (i % 10) * 40;
    const y = 76 + Math.floor(i / 10) * 40;
    const on = lit.includes(i);
    if (moving && movers.has(i)) {
      cells.push(`<circle cx="${x}" cy="${y}" r="13" class="turn" style="--rest:${on ? 1 : 0.423};--delay:${movers.get(i)}s"/>`);
    } else {
      cells.push(`<circle cx="${x}" cy="${y}" r="${on ? 13 : 5.5}"/>`);
    }
  }
  return `<svg class="mark" viewBox="0 0 512 512" width="${size}" height="${size}" role="img" aria-label="mia: a hundred dots, a few of them lit">${cells.join("")}</svg>`;
};

const small = (size: number) => {
  const cells: string[] = [];
  for (let i = 0; i < 16; i++) {
    const x = 88 + (i % 4) * 112;
    const y = 88 + Math.floor(i / 4) * 112;
    cells.push(`<circle cx="${x}" cy="${y}" r="${[1, 7, 12].includes(i) ? 46 : 20}"/>`);
  }
  return `<svg class="mark small" viewBox="0 0 512 512" width="${size}" height="${size}" aria-hidden="true">${cells.join("")}</svg>`;
};

const icon = {
  github: `<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M12 .5a11.5 11.5 0 0 0-3.64 22.41c.58.1.79-.25.79-.56v-2c-3.2.7-3.88-1.37-3.88-1.37-.53-1.33-1.28-1.69-1.28-1.69-1.05-.71.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.1-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.17 1.18a11 11 0 0 1 5.77 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.84 1.19 3.1 0 4.42-2.7 5.4-5.26 5.68.41.36.78 1.06.78 2.14v3.17c0 .31.21.67.8.56A11.5 11.5 0 0 0 12 .5Z"/></svg>`,
  search: `<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg>`,
  theme: `<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/></svg>`,
};

function topbar(active: "home" | "docs"): string {
  return `<header class="topbar">
  <a class="brand" href="/">${small(18)}<span>mia</span></a>
  <nav>
    <button class="search-open" type="button" aria-label="Search the docs">${icon.search}<span>Search</span><kbd>⌘K</kbd></button>
    <a class="link${active === "docs" ? " here" : ""}" href="/docs/">Docs</a>
    <a class="link" href="${repo}" rel="noopener">${icon.github}<span>GitHub</span></a>
    <button class="theme" type="button" aria-label="Light or dark">${icon.theme}</button>
  </nav>
</header>`;
}

const footer = `<footer class="foot">
  <span><em lang="sw">mia</em> · Swahili for 100</span>
  <span><a href="/docs/">Docs</a><a href="/docs/plugins">Plugins</a><a href="${repo}" rel="noopener">GitHub</a><a href="${repo}/blob/main/LICENSE" rel="noopener">MIT</a></span>
</footer>`;

const searchDialog = `<dialog class="search" aria-label="Search">
  <form method="dialog"><input type="search" placeholder="Search the docs" autocomplete="off" spellcheck="false" aria-label="Search the docs"></form>
  <ol class="results"></ol>
  <p class="hint"><kbd>↑</kbd><kbd>↓</kbd> choose <kbd>⏎</kbd> open <kbd>esc</kbd> close</p>
</dialog>`;

function page(opts: { title: string; description: string; body: string; active: "home" | "docs"; bodyClass: string }): string {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escape(opts.title)}</title>
<meta name="description" content="${escape(opts.description)}">
<meta property="og:title" content="${escape(opts.title)}">
<meta property="og:description" content="${escape(opts.description)}">
<link rel="icon" href="/mark.svg" type="image/svg+xml">
<link rel="apple-touch-icon" href="/app-icon.png">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Manrope:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500;600&display=swap">
<link rel="stylesheet" href="/site.css?v=${version}">
<script>try{const t=localStorage.getItem("mia-theme");if(t)document.documentElement.dataset.theme=t}catch{}</script>
</head>
<body class="${opts.bodyClass}">
${topbar(opts.active)}
${opts.body}
${footer}
${searchDialog}
<script src="/site.js?v=${version}" defer></script>
</body>
</html>
`;
}

type Built = (typeof pages)[number] & ReturnType<typeof render>;

function sidebar(current: string): string {
  const groups = [...new Set(pages.map((p) => p.group))];
  return `<nav class="sidebar" aria-label="Documentation">
${groups
  .map(
    (group) => `<p class="eyebrow">${group}</p><ul>${pages
      .filter((p) => p.group === group)
      .map((p) => `<li><a href="/docs/${p.slug}"${p.slug === current ? ' aria-current="page"' : ""}>${built.get(p.slug)!.title}</a></li>`)
      .join("")}</ul>`,
  )
  .join("\n")}
</nav>`;
}

function outline(headings: Heading[]): string {
  const shown = headings.filter((h) => h.depth === 2);
  if (shown.length < 2) return "";
  return `<aside class="outline" aria-label="On this page"><p class="eyebrow">On this page</p><ul>${shown
    .map((h) => `<li><a href="#${h.id}">${escape(h.text)}</a></li>`)
    .join("")}</ul></aside>`;
}

const built = new Map<string, Built>();
for (const p of pages) {
  const markdown = await readFile(join(docs, p.file), "utf8");
  built.set(p.slug, { ...p, ...render(markdown) });
}

await rm(out, { recursive: true, force: true });
await mkdir(join(out, "docs"), { recursive: true });

const index: { url: string; page: string; heading: string; text: string }[] = [];

function plain(markdown: string): string {
  return markdown
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/[`*_>#|]/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function sections(markdown: string): Map<string, string> {
  const bySlug = new Map<string, string>();
  const seen = new Map<string, number>();
  let current = "";
  let buffer: string[] = [];
  const flush = () => bySlug.set(current, plain(buffer.join("\n")).slice(0, 2000));
  let fenced = false;
  for (const line of markdown.split("\n")) {
    if (line.startsWith("```")) fenced = !fenced;
    const m = !fenced && /^(#{2,6})\s+(.*)$/.exec(line);
    if (m) {
      flush();
      let id = slugify(m[2]);
      const n = seen.get(id) ?? 0;
      seen.set(id, n + 1);
      if (n) id = `${id}-${n}`;
      current = id;
      buffer = [];
    } else buffer.push(line);
  }
  flush();
  return bySlug;
}

for (const [i, p] of pages.entries()) {
  const b = built.get(p.slug)!;
  const prev = pages[i - 1] && built.get(pages[i - 1].slug)!;
  const next = pages[i + 1] && built.get(pages[i + 1].slug)!;
  const nav = `<nav class="pager">${prev ? `<a class="prev" href="/docs/${prev.slug}"><span>Previous</span>${prev.title}</a>` : "<span></span>"}${
    next ? `<a class="next" href="/docs/${next.slug}"><span>Next</span>${next.title}</a>` : ""
  }</nav>`;
  const body = `<div class="docs">
${sidebar(p.slug)}
<main class="prose">
${b.html}
<p class="edit"><a href="${repo}/blob/main/docs/${p.file}" rel="noopener">Edit this page</a></p>
${nav}
</main>
${outline(b.headings)}
</div>`;
  await writeFile(
    join(out, "docs", `${p.slug}.html`),
    page({ title: `${b.title} · mia`, description: b.lead, body, active: "docs", bodyClass: "doc" }),
  );
  const text = sections(await readFile(join(docs, p.file), "utf8"));
  index.push({ url: `/docs/${p.slug}`, page: b.title, heading: b.title, text: text.get("") ?? "" });
  for (const h of b.headings) index.push({ url: `/docs/${p.slug}#${h.id}`, page: b.title, heading: h.text, text: text.get(h.id) ?? "" });
}

const contents = render(await readFile(join(docs, "README.md"), "utf8"));
await writeFile(
  join(out, "docs", "index.html"),
  page({
    title: "Documentation · mia",
    description: "Everything mia does, from your first worktree to writing a plugin.",
    body: `<div class="docs">
${sidebar("")}
<main class="prose">${contents.html}</main>
</div>`,
    active: "docs",
    bodyClass: "doc",
  }),
);

const sample = (text: string, lang = "bash") => code(text, lang);
const home = `<main class="home">
<section class="hero">
  ${mark(220, true)}
  <div class="definition" lang="en">
    <p class="definition-head"><strong lang="sw">mia</strong><span class="say" aria-label="pronounced MEE-ah">/ˈmi.a/</span><em>number</em><span class="from">Swahili</span></p>
    <p class="definition-sense">100.</p>
  </div>
  <a class="cue" href="#what">What it does <span aria-hidden="true">↓</span></a>
</section>

<section class="pitch" id="what">
  <h1>Work on 100 things at once.</h1>
  <p>Every piece of work gets its own worktree, tmux session and name, and its own container at <code>https://&lt;name&gt;.mia</code> when it needs one. Build and test them side by side, here or on any machine you can ssh to. Everything else is a plugin.</p>
  <div class="install">${sample("go install github.com/emusoi/mia-core/cmd/mia@latest")}</div>
  <p class="actions"><a class="pill" href="/docs/getting-started">Get started</a><a class="quiet" href="/docs/why">Why mia</a></p>
</section>

<section class="features">
  <article class="wide">
    <div>
    <p class="eyebrow">The dashboard</p>
    <h2>One list, grouped by what needs you.</h2>
    <p>The main checkout, what is in progress, what has gone quiet. Every key is a <code>mia</code> command, and <code>?</code> shows which. Pop it over tmux with one key.</p>
    <a href="/docs/dashboard">The dashboard →</a>
    </div>
    <pre class="frame" aria-label="The mia dashboard">╭── Worktrees / shop ─────────────────────────╮
│                                             │
│   main  1                                   │
│   shop       main                     2h    │
│                                             │
│   in progress  2                            │
│ ± monduli    due-dates   uncommitted  12m   │
│   kijenge    search      +1 −4         3h   │
│                                             │
│   quiet  4                                  │
╰── ⏎ attach   n new   / find   ? keys ───────╯</pre>
  </article>
  <article>
    <p class="eyebrow">Worktrees</p>
    <h2>Every branch, a place with a name.</h2>
    <p><code>mia new due-dates</code> makes a worktree beside your repository and names it — <code>monduli</code> — for life. Its tmux session keeps running when you leave.</p>
    ${sample("mia new due-dates\nmia shell monduli\nmia switch monduli")}
    <a href="/docs/worktrees">Worktrees →</a>
  </article>
  <article>
    <p class="eyebrow">Stacks</p>
    <h2>One feature, several pull requests.</h2>
    <p>Layers share one worktree, so <code>mia up</code> and <code>mia down</code> move between them in place. mia never merges into your base and never pushes — it prints the commands.</p>
    ${sample("mia new schema --stack\nmia new api --stack\nmia stack pr")}
    <a href="/docs/stacks">Stacks →</a>
  </article>
  <article>
    <p class="eyebrow">Environments</p>
    <h2>localhost, with a name.</h2>
    <p>A container per worktree, from what the project already says. Whatever you would type on <code>localhost:5173</code>, type <code>monduli.mia:5173</code> — here, or on any machine you can ssh to.</p>
    ${sample("mia env up\nmia machine add build me@build.example.com\nmia env host build")}
    <a href="/docs/environments">Environments →</a>
  </article>
  <article>
    <p class="eyebrow">Plugins</p>
    <h2>The rest is yours.</h2>
    <p>Plans, agents, editors, pull request status: a plugin is any program named <code>mia-&lt;name&gt;</code>. It can add verbs, rows, sections, keys, panels and listen for events.</p>
    ${sample('#!/bin/sh\ncase "$1" in\nmanifest) echo \'{"protocol":1,"verbs":[{"name":"hello"}]}\' ;;\nhello) echo "hello from $MIA_WORKTREE" ;;\nesac')}
    <a href="/docs/plugins">Writing a plugin →</a>
  </article>
  <article class="span">
    <p class="eyebrow">Nothing in your tree</p>
    <h2>Your repository stays yours.</h2>
    <p>mia writes nothing into the working tree; everything it keeps is in <code>.git/mia</code>. The one exception is <code>mia dev</code>, when you ask it to lend main\'s dev server a branch, and it gives main its files back. It keeps no state it could lose, either: routes, sessions and names are worked out from git, tmux and the container engine every time.</p>
    <a href="/docs/getting-started">Getting started →</a>
  </article>
</section>
</main>`;

await writeFile(
  join(out, "index.html"),
  page({
    title: "mia — work on 100 things at once",
    description: "Every piece of work gets its own worktree, tmux session and name, and its own container at <name>.mia. Build and test them side by side, here or on any machine you can ssh to. Mia is Swahili for 100.",
    body: home,
    active: "home",
    bodyClass: "front",
  }),
);

await writeFile(join(out, "search.json"), JSON.stringify(index));
await copyFile(join(root, "design", "exports", "mark-small.svg"), join(out, "mark.svg"));
await copyFile(join(root, "design", "exports", "app-icon@2x.png"), join(out, "app-icon.png"));
await copyFile(join(import.meta.dir, "site.css"), join(out, "site.css"));
await copyFile(join(import.meta.dir, "site.js"), join(out, "site.js"));
await writeFile(
  join(out, "404.html"),
  page({
    title: "Not here · mia",
    description: "No page here.",
    body: `<main class="home"><section class="hero">${mark(120)}<p class="lost">Nothing at this address. <a href="/docs/">The docs</a> are here.</p></section></main>`,
    active: "home",
    bodyClass: "front",
  }),
);

console.log(`built ${pages.length + 3} pages into ${out}`);
