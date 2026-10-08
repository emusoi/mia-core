const version = document.currentScript?.src.split("v=")[1] ?? "";
const root = document.documentElement;

document.querySelector(".theme")?.addEventListener("click", () => {
  const dark = root.dataset.theme
    ? root.dataset.theme === "dark"
    : matchMedia("(prefers-color-scheme: dark)").matches;
  root.dataset.theme = dark ? "light" : "dark";
  try { localStorage.setItem("mia-theme", root.dataset.theme); } catch {}
});

for (const button of document.querySelectorAll(".copy")) {
  button.addEventListener("click", async () => {
    const text = button.parentElement.querySelector("pre").innerText.replace(/\n$/, "");
    try {
      await navigator.clipboard.writeText(text);
      button.textContent = "Copied";
      button.classList.add("done");
      setTimeout(() => { button.textContent = "Copy"; button.classList.remove("done"); }, 1400);
    } catch {}
  });
}

const outline = [...document.querySelectorAll(".outline a")];
if (outline.length) {
  const byId = new Map(outline.map((a) => [a.hash.slice(1), a]));
  const seen = new IntersectionObserver((entries) => {
    for (const entry of entries) {
      if (!entry.isIntersecting) continue;
      outline.forEach((a) => a.classList.remove("here"));
      byId.get(entry.target.id)?.classList.add("here");
    }
  }, { rootMargin: "0px 0px -70% 0px" });
  for (const id of byId.keys()) {
    const heading = document.getElementById(id);
    if (heading) seen.observe(heading);
  }
}

const dialog = document.querySelector(".search");
const input = dialog?.querySelector("input");
const results = dialog?.querySelector(".results");
let index = null;
let chosen = 0;

async function openSearch() {
  if (!dialog || dialog.open) return;
  dialog.showModal();
  input.value = "";
  results.innerHTML = "";
  if (!index) {
    try { index = await (await fetch(`/search.json?v=${version}`)).json(); } catch { index = []; }
    show(input.value);
  }
}

function show(query) {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  results.innerHTML = "";
  if (!words.length || !index) return;
  const found = index
    .map((entry) => {
      const title = `${entry.heading} ${entry.page}`.toLowerCase();
      const body = entry.text.toLowerCase();
      if (!words.every((w) => title.includes(w) || body.includes(w))) return null;
      const inTitle = words.filter((w) => title.includes(w)).length;
      const score = (words.length - inTitle) * 2 + (entry.heading.toLowerCase().startsWith(words[0]) ? 0 : 1);
      const at = body.indexOf(words.find((w) => body.includes(w)) ?? "\u0000");
      const snippet = inTitle === words.length || at < 0 ? "" : (at > 40 ? "…" : "") + entry.text.slice(Math.max(0, at - 40), at + 90) + "…";
      return { entry, score, snippet };
    })
    .filter(Boolean)
    .sort((a, b) => a.score - b.score)
    .slice(0, 12);
  chosen = 0;
  for (const [i, { entry, snippet }] of found.entries()) {
    const li = document.createElement("li");
    const a = document.createElement("a");
    a.href = entry.url;
    a.textContent = entry.heading;
    const where = document.createElement("span");
    where.textContent = [entry.heading !== entry.page ? entry.page : "", snippet].filter(Boolean).join(" · ");
    if (where.textContent) a.append(where);
    a.setAttribute("aria-selected", String(i === 0));
    li.append(a);
    results.append(li);
  }
}

function choose(step) {
  const links = [...results.querySelectorAll("a")];
  if (!links.length) return;
  chosen = (chosen + step + links.length) % links.length;
  links.forEach((a, i) => a.setAttribute("aria-selected", String(i === chosen)));
  links[chosen].scrollIntoView({ block: "nearest" });
}

document.querySelector(".search-open")?.addEventListener("click", openSearch);
input?.addEventListener("input", () => show(input.value));
input?.addEventListener("keydown", (event) => {
  if (event.key === "ArrowDown") { event.preventDefault(); choose(1); }
  if (event.key === "ArrowUp") { event.preventDefault(); choose(-1); }
  if (event.key === "Enter") {
    event.preventDefault();
    const link = results.querySelectorAll("a")[chosen];
    if (link) { dialog.close(); location.href = link.href; }
  }
});
dialog?.addEventListener("click", (event) => { if (event.target === dialog) dialog.close(); });

document.addEventListener("keydown", (event) => {
  const typing = /^(INPUT|TEXTAREA)$/.test(document.activeElement?.tagName ?? "");
  if ((event.key === "k" && (event.metaKey || event.ctrlKey)) || (event.key === "/" && !typing)) {
    event.preventDefault();
    openSearch();
  }
});
