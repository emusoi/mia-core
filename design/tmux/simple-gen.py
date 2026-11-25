# mia in tmux, simpler: one key and one place to type, drawn three ways, four moments each
# (go somewhere, start an agent on a task, move the conversation, answer who asks).
#   python3 simple-gen.py            rebuild these pages in tmux.buni, shoot every page, join them as simple.pdf
#
# A · One line: prefix k turns the status line into a prompt. No popup at all.
# B · One list: prefix k opens one list of everything; typing narrows it, Enter does it.
# C · One shell: the popup terminal that exists, its verbs cut to a word each.
import importlib.util, os, re, subprocess, sys

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("tmuxgen", HERE + "/tmux-gen.py")
T = importlib.util.module_from_spec(spec); spec.loader.exec_module(T)
# (Pages go into tmux.buni beside the first designs: it holds the tmux client they are drawn for.)
OUT = "/private/tmp/claude-501/applab/out/tmux-simple"
COLS, ROWS, INK, MUTED, DIM, AMBER, GREEN, GROUND = T.COLS, T.ROWS, T.INK, T.MUTED, T.DIM, T.AMBER, T.GREEN, T.GROUND
ASK = "The ntfy topic file is missing. Make a new topic?"
OPTS = ["make one", "use the old topic", "stop here"]


# ── A · One line ───────────────────────────────────────────────────────────────────────────
def a(typed, ghost="", above=None):
    s = T.a_base()
    y = ROWS - 1
    s.fill(0, y, COLS, 1, GROUND)
    x = s.segs(1, y, [("mia", MUTED), (" ❯ ", DIM), (typed, INK)])
    s.put(x, y, " ", GROUND, INK)
    s.put(x + 1, y, ghost, DIM)
    if above:
        s.fill(0, y - 1, COLS, 1, GROUND)
        s.segs(1, y - 1, above)
    return s

def a1(): return a("sin", "za   mia-core · user/radar-answers-numbered-by-age")
def a2(): return a("pi fix the flaky upload test", "   ⏎ starts pi in goba on it")
def a3(): return a("> cod", "ex   ⏎ moves this conversation to codex · gpt-5 · high")
def a4():
    opts = [(f"   {i + 1} ", INK) if j == 0 else (o, MUTED) for i, o in enumerate(OPTS) for j in (0, 1)]
    return a("1", "   ⏎ answers themi: make one", [("●", AMBER), (" themi asks  ", AMBER), (ASK, INK)] + opts)


# ── B · One list ───────────────────────────────────────────────────────────────────────────
def b(typed, rows, sel=0):
    """rows: (mark, mark colour, words, right) or a str heading."""
    s = T.a_base()
    w, h = 84, len(rows) + 4
    x, y = (COLS - w) // 2, (ROWS - 1 - h) // 2
    s.box(x, y, w, h, MUTED, GROUND, "mia", MUTED)
    end = s.segs(x + 2, y + 1, [("❯ ", DIM), (typed, INK)])
    s.put(end, y + 1, " ", GROUND, INK)
    n = 0
    for i, r in enumerate(rows):
        yy = y + 3 + i
        if isinstance(r, str):
            s.put(x + 2, yy, r, DIM)
            continue
        mark, colour, words, right = r
        s.put(x + 2, yy, mark, colour)
        s.put(x + 4, yy, words, INK)
        s.put(x + w - 2 - len(right), yy, right, MUTED)
        if n == sel:
            s.reverse(x + 1, yy, w - 2)
        n += 1
    return s

def b1(): return b("", [
    ("●", AMBER, "themi    " + ASK, "mia-phone"),
    ("●", AMBER, "sinza    Run the migration against the staging database now?", "lido-app"),
    ("⠹", DIM, "goba     user/foot-width", "mia-core"),
    ("⠹", DIM, "tabata   user/cd-by-branch", "mia-core"),
    (" ", DIM, "kijenge  user/popup-size", "mia-core")])
def b2(): return b("sin", [
    (" ", DIM, "sinza    user/radar-answers-numbered-by-age", "mia-core"),
    ("●", AMBER, "sinza    Run the migration against the staging database now?", "lido-app")])
def b3(): return b("pi fix the flaky upload test", [
    ("+", GREEN, "start pi in goba on: fix the flaky upload test", "new window"),
    ("+", GREEN, "start claude in goba on it", "new window"),
    ("+", GREEN, "start pi in a new worktree on it", "new worktree")])
def b4(): return b("> ", [
    ("→", MUTED, "move this conversation to codex", "gpt-5 · high"),
    ("→", MUTED, "move this conversation to opencode", "sonnet"),
    ("→", MUTED, "move this conversation to pi", "default"),
    ("→", MUTED, "move this conversation to agy", "gemini")])


# ── C · One shell ──────────────────────────────────────────────────────────────────────────
def c(lines):
    s = T.a_base()
    w, h = 84, 16
    x, y = (COLS - w) // 2, (ROWS - 1 - h) // 2
    s.box(x, y, w, h, MUTED, GROUND, "mia", MUTED)
    for i, parts in enumerate(lines):
        end = s.segs(x + 2, y + 1 + i, parts)
    s.put(end, y + len(lines), " ", GROUND, INK)
    return s

P = lambda cmd, at="mia-core.goba": [(at, MUTED), (" ❯ ", DIM), (cmd, INK)]
O = lambda text, colour=INK: [(text, colour)]
def c1(): return c([P("ls")] + [O(T.tree_line(*t)) for t in T.TREES] + [P("cd sin")])
def c2(): return c([P("pi fix the flaky upload test"), O("goba: pi started on it, window 5", MUTED), P("")])
def c3(): return c([P("hand codex"), O("goba: this conversation goes on with codex, window 6", MUTED), P("")])
def c4(): return c([P("asks"), O("1  themi  mia-phone  claude  12m"), O("   " + ASK, MUTED)]
                   + [O(f"     {i + 1}  {o}", MUTED) for i, o in enumerate(OPTS)]
                   + [O("2  sinza  lido-app  codex  4m"), O("   Run the migration against the staging database now?", MUTED), P("1 1")])


PAGES = [("A1", "A1 · One line: go somewhere", a1), ("A2", "A2 · One line: start an agent on a task", a2),
         ("A3", "A3 · One line: move the conversation", a3), ("A4", "A4 · One line: answer who asks", a4),
         ("B1", "B1 · One list: opened, who asks first", b1), ("B2", "B2 · One list: go somewhere", b2),
         ("B3", "B3 · One list: start an agent on a task", b3), ("B4", "B4 · One list: move the conversation", b4),
         ("C1", "C1 · One shell: go somewhere", c1), ("C2", "C2 · One shell: start an agent on a task", c2),
         ("C3", "C3 · One shell: move the conversation", c3), ("C4", "C4 · One shell: answer who asks", c4)]

if __name__ == "__main__":
    T.PAGES = PAGES
    T.build(set())
    os.makedirs(OUT, exist_ok=True)
    now = T.pages_now()
    from PIL import Image
    shots = []
    for key, name, _ in PAGES:
        png = f"{OUT}/{key}.png"
        subprocess.run(["buni", "shot", T.FILE, now[name][0], png], check=True, capture_output=True)
        shots.append(Image.open(png).convert("RGB"))
    shots[0].save(OUT + "/simple.pdf", save_all=True, append_images=shots[1:])
    print(OUT + "/simple.pdf")
