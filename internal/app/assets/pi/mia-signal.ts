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
//
// And it writes what pi has not yet written of its session, for mia to read
// as pi writes it: pi writes a message to its session file only once it is
// done, and nothing of a new session until its first reply is. Next to the
// session file, under its name with .live after it, at most every 150 ms
// while a reply is written and replaced whole each time, is
//
//   {"size": <how long the session file is, -1 while it is not there>}
//   <each line pi has kept but not written, of a session not yet there>
//   <the reply as far as it is written: {"type":"message","live":true,...}>
//
// and once pi has written it all the file is gone. It is the one file this
// writes, and nothing that goes wrong with it reaches pi.

import { spawn } from "node:child_process";
import { renameSync, rmSync, statSync, writeFileSync } from "node:fs";

interface Message {
	role?: string;
	content?: unknown;
	timestamp?: number;
}

interface Event {
	aborted?: boolean;
	title?: string;
	message?: Message;
}

interface Sessions {
	getSessionFile(): string | undefined;
	getHeader(): unknown;
	getEntries(): unknown[];
}

interface Context {
	cwd?: string;
	hasUI?: boolean;
	sessionManager?: Sessions;
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
const liveEvery = 150; // ms at most between two writes of what pi has not yet written

// unwritten is a message pi is still writing as a line of its session would
// have it, with only what is read of it.
const unwritten = (message: Message): unknown => ({
	type: "message",
	live: true,
	timestamp: new Date(typeof message.timestamp === "number" ? message.timestamp : Date.now()).toISOString(),
	message: {
		role: message.role,
		content: Array.isArray(message.content)
			? message.content.map((b: Record<string, unknown>) => ({
					type: b?.type,
					text: b?.text,
					thinking: b?.thinking,
					redacted: b?.redacted,
					name: b?.name,
					arguments: b?.arguments,
				}))
			: message.content,
	},
});

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

	// What pi has not yet written. pi writes a message only after every
	// handler of its message_end has run: a write here waits liveEvery, and
	// by then it has.
	let session: Sessions | undefined;
	let writing: Message | undefined; // the reply pi is writing
	let wrote = ""; // the live file, while there is one
	let soon: ReturnType<typeof setTimeout> | undefined;
	const forget = (): void => {
		if (!wrote) return;
		try {
			rmSync(wrote, { force: true });
		} catch {}
		wrote = "";
	};
	const write = (): void => {
		soon = undefined;
		let next = "";
		try {
			const file = session?.getSessionFile();
			if (!session || !file) return; // a session pi keeps no file of
			let size = -1;
			try {
				size = statSync(file).size;
			} catch {}
			const lines: unknown[] = [];
			if (size < 0) {
				const entries = session.getEntries();
				if (entries.some((e) => (e as { type?: string })?.type === "message")) lines.push(session.getHeader(), ...entries);
			}
			if (writing) lines.push(unwritten(writing));
			if (lines.length === 0) return forget();
			const live = `${file}.live`;
			next = `${live}.${process.pid}`;
			writeFileSync(next, `${[{ size }, ...lines].map((line) => JSON.stringify(line)).join("\n")}\n`);
			renameSync(next, live);
			next = "";
			wrote = live;
		} catch {
			if (next) {
				try {
					rmSync(next, { force: true });
				} catch {}
			}
		}
	};
	const later = (ctx: Context): void => {
		session = ctx?.sessionManager ?? session;
		if (soon !== undefined) return;
		soon = setTimeout(write, liveEvery);
		soon.unref?.();
	};

	on("message_start", (e, ctx) => {
		if (e?.message?.role === "assistant") writing = e.message;
		later(ctx);
	});
	on("message_update", (e, ctx) => {
		if (e?.message?.role === "assistant") writing = e.message;
		later(ctx);
	});
	on("message_end", (e, ctx) => {
		if (e?.message?.role === "assistant") writing = undefined;
		later(ctx);
	});

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
		clearTimeout(soon);
		soon = undefined;
		writing = undefined;
		forget();
		clearTimeout(ending);
		if (running) call(ctx, clear);
		running = asking = false;
	});
}
