// mia-signal: written by mia — `mia account signals pi` puts it here and writes over it; a file without this line it leaves alone.
//
// Tells mia what pi does in one of mia's worktrees, as claude's hooks do,
// with `mia signal .` and nothing else:
//
//   busy   a run began                                  agent_start
//   asks   an extension's dialog waits for you, in a run  ui_prompt_start … ui_prompt_end
//   done   pi goes on with nothing by itself            agent_settled, or agent_end on a pi that has none
//
// A run you stopped clears pi's signal. Where there is no mia, or the folder
// is no worktree of mia's, the first call says so and no other is made. No
// call is waited for by pi, each is ended after three seconds, and nothing
// here throws into pi.

import { spawn } from "node:child_process";

interface Event {
	aborted?: boolean;
	title?: string;
}

interface Context {
	cwd?: string;
	hasUI?: boolean;
}

interface Pi {
	on(event: string, handler: (event: Event, ctx: Context) => void): unknown;
}

const from = ["--from", "pi"];
const busy = ["busy", "--style", "dim", "--rank", "60", ...from];
const done = ["done", "--style", "green", "--rank", "30", ...from, "--until", "seen"];
const clear = ["clear", ...from];
const callFor = 3000; // ms a call has
const settleFor = 1500; // ms after agent_end with no other run, on a pi with no agent_settled

export default function (pi: Pi): void {
	let off = false; // no mia, or no worktree of mia's
	let first = true;
	let running = false;
	let asking = false;
	let settles = false; // this pi says agent_settled
	let said = ""; // the kind last said
	let ending: ReturnType<typeof setTimeout> | undefined;
	let last: Promise<void> = Promise.resolve();

	// call runs `mia signal . <args>` after the call before it, so two said
	// close together arrive in order.
	const call = (ctx: Context, args: string[]): void => {
		if (off || ctx?.hasUI === false) return; // print and json modes: nobody is there to call
		if (args[0] === "busy" && said === "busy") return; // another run of the same work
		said = args[0];
		const cwd = ctx?.cwd || process.cwd();
		last = last.then(
			() =>
				new Promise<void>((resolve) => {
					if (off) return resolve();
					let over = false;
					const end = (): void => {
						if (over) return;
						over = true;
						first = false;
						clearTimeout(late);
						resolve();
					};
					const late = setTimeout(() => {
						try {
							child?.kill("SIGKILL");
						} catch {}
						end();
					}, callFor);
					late.unref?.();
					let child: ReturnType<typeof spawn> | undefined;
					try {
						child = spawn("mia", ["signal", ".", ...args], { cwd, stdio: "ignore" });
						child.on("error", () => {
							off = true; // mia is not on PATH, or cannot be run
							end();
						});
						child.on("exit", (code) => {
							if (first && typeof code === "number" && code !== 0) off = true; // no worktree of mia's
							end();
						});
						child.unref();
					} catch {
						off = true;
						end();
					}
				}),
		);
	};

	const on = (event: string, handler: (event: Event, ctx: Context) => void): void => {
		try {
			pi.on(event, (e, ctx) => {
				try {
					handler(e, ctx);
				} catch {}
			});
		} catch {}
	};

	const finish = (ctx: Context, aborted: boolean): void => {
		clearTimeout(ending);
		running = asking = false;
		call(ctx, aborted ? clear : done);
	};

	on("agent_start", (_e, ctx) => {
		clearTimeout(ending);
		running = true;
		asking = false;
		call(ctx, busy);
	});
	// agent_end is not the end: pi may try again, compact, or go on with what
	// is queued. agent_settled is, where pi has it.
	on("agent_end", (_e, ctx) => {
		if (settles) return;
		clearTimeout(ending);
		ending = setTimeout(() => {
			try {
				finish(ctx, false);
			} catch {}
		}, settleFor);
		ending.unref?.();
	});
	on("agent_settled", (e, ctx) => {
		settles = true;
		finish(ctx, e?.aborted === true);
	});
	on("ui_prompt_start", (e, ctx) => {
		if (!running) return; // a dialog you opened yourself
		asking = true;
		const title = String(e?.title ?? "").replace(/\s+/g, " ").replace(/^[-\s]+/, "").slice(0, 200);
		call(ctx, ["asks", title || "asks", "--style", "red", "--rank", "10", ...from]);
	});
	on("ui_prompt_end", (_e, ctx) => {
		if (!asking) return;
		asking = false;
		if (running) call(ctx, busy);
	});
	on("session_shutdown", (_e, ctx) => {
		clearTimeout(ending);
		if (running) call(ctx, clear);
		running = asking = false;
	});
}
