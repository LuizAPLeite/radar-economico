// Roteiro de capturas no Edge headless, via Chrome DevTools Protocol.
// Uso: node shoot.mjs roteiro.json
//
// roteiro.json: { "width": 1280, "height": 800, "scale": 1, "scheme": "light",
//   "steps": [ {"goto": url}, {"wait": ms}, {"waitFor": "expressão JS"},
//              {"scrollTo": "seletor CSS" | número}, {"click": "texto do botão"},
//              {"type": "texto", "into": "seletor", "frameEvery": n, "frameMs": ms},
//              {"key": "Enter"}, {"shot": "arquivo.png", "fullPage": bool},
//              {"frame": ms} ],
//   "framesDir": "pasta" }
// "frame" salva o que está na tela como o próximo quadro do GIF, que fica
// `ms` milissegundos na tela; a lista vai para framesDir/frames.txt (formato
// concat do ffmpeg).
import { spawn } from "node:child_process";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const EDGE = "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe";
const plan = JSON.parse(await readFile(process.argv[2], "utf8"));
const port = 9334;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const edge = spawn(
  EDGE,
  [
    "--headless=new",
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${join(tmpdir(), "radar-shoot-profile")}`,
    "--enable-unsafe-swiftshader",
    "--hide-scrollbars",
    "--no-first-run",
    "--lang=pt-BR",
    "about:blank",
  ],
  { stdio: "ignore" },
);

const frames = [];
let frameNo = 0;

try {
  let page;
  for (let i = 0; i < 50 && !page; i++) {
    try {
      page = (await (await fetch(`http://127.0.0.1:${port}/json/list`)).json()).find((t) => t.type === "page");
    } catch {
      await sleep(200);
    }
  }
  if (!page) throw new Error("Edge não abriu a porta de depuração");

  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((r) => ws.addEventListener("open", r, { once: true }));
  let seq = 0;
  const pending = new Map();
  const listeners = [];
  ws.addEventListener("message", (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.id && pending.has(msg.id)) {
      const { resolve, reject } = pending.get(msg.id);
      pending.delete(msg.id);
      msg.error ? reject(new Error(msg.error.message)) : resolve(msg.result);
    } else listeners.forEach((fn) => fn(msg));
  });
  const send = (method, params = {}) =>
    new Promise((resolve, reject) => {
      const id = ++seq;
      pending.set(id, { resolve, reject });
      ws.send(JSON.stringify({ id, method, params }));
    });
  const evaluate = async (expression) =>
    (await send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true })).result.value;

  const metrics = (height) =>
    send("Emulation.setDeviceMetricsOverride", {
      width: plan.width,
      height,
      deviceScaleFactor: plan.scale ?? 1,
      mobile: false,
    });

  await send("Page.enable");
  await metrics(plan.height);
  await send("Emulation.setEmulatedMedia", {
    features: [{ name: "prefers-color-scheme", value: plan.scheme ?? "light" }],
  });
  if (plan.framesDir) await mkdir(plan.framesDir, { recursive: true });

  const frame = async (ms) => {
    const shot = await send("Page.captureScreenshot", { format: "png" });
    const name = `f${String(++frameNo).padStart(4, "0")}.png`;
    await writeFile(join(plan.framesDir, name), Buffer.from(shot.data, "base64"));
    frames.push({ name, ms });
  };

  for (const step of plan.steps) {
    if (step.goto) {
      const loaded = new Promise((r) => listeners.push((m) => m.method === "Page.loadEventFired" && r()));
      await send("Page.navigate", { url: step.goto });
      await loaded;
      await sleep(1500);
    } else if (step.wait) {
      await sleep(step.wait);
    } else if (step.waitFor) {
      for (let i = 0; i < 240 && !(await evaluate(step.waitFor)); i++) await sleep(250);
      await sleep(800);
    } else if (step.scrollTo !== undefined) {
      const target = typeof step.scrollTo === "number"
        ? `window.scrollTo({top: ${step.scrollTo}, behavior: "instant"})`
        : `document.querySelector(${JSON.stringify(step.scrollTo)})?.scrollIntoView({block: "start", behavior: "instant"}); window.scrollBy(0, -16)`;
      await evaluate(target);
      await sleep(600);
    } else if (step.scrollToText) {
      const ok = await evaluate(
        `(() => { const e = [...document.querySelectorAll("h1, h2, h3")].find(e => e.textContent.trim().startsWith(${JSON.stringify(step.scrollToText)})); if (!e) return false; e.scrollIntoView({block: "start", behavior: "instant"}); window.scrollBy(0, -(${step.offset ?? 40})); return true })()`,
      );
      if (!ok) throw new Error(`não achei o título "${step.scrollToText}"`);
      await sleep(600);
    } else if (step.click) {
      const ok = await evaluate(
        `(() => { const b = [...document.querySelectorAll("button, a")].find(e => e.textContent.trim().startsWith(${JSON.stringify(step.click)})); b?.click(); return !!b })()`,
      );
      if (!ok) throw new Error(`não achei o botão "${step.click}"`);
    } else if (step.type !== undefined) {
      await evaluate(`document.querySelector(${JSON.stringify(step.into)}).focus()`);
      const every = step.frameEvery ?? 0;
      for (const [i, ch] of [...step.type].entries()) {
        await send("Input.insertText", { text: ch });
        await sleep(25);
        if (every && (i + 1) % every === 0) await frame(step.frameMs ?? 80);
      }
    } else if (step.key) {
      for (const type of ["keyDown", "keyUp"]) {
        await send("Input.dispatchKeyEvent", {
          type,
          key: step.key,
          code: step.key,
          windowsVirtualKeyCode: step.key === "Enter" ? 13 : 0,
        });
      }
    } else if (step.shot) {
      // Altura própria da foto (o WebGL e os gráficos só desenham o que está
      // na área visível): a janela cresce, espera redesenhar e volta.
      let height = step.height;
      if (step.fullPage) height = Math.ceil((await send("Page.getLayoutMetrics")).cssContentSize.height);
      if (height) {
        await metrics(height);
        await sleep(2500);
      }
      const shot = await send("Page.captureScreenshot", { format: "png" });
      await writeFile(step.shot, Buffer.from(shot.data, "base64"));
      if (height) await metrics(plan.height);
      console.log("salvo:", step.shot);
    } else if (step.frame) {
      await frame(step.frame);
    }
  }

  if (frames.length) {
    // Formato concat do ffmpeg: cada quadro com a sua duração; o último se
    // repete no fim (exigência do formato para a duração dele valer).
    const lines = frames.flatMap((f) => [`file '${f.name}'`, `duration ${(f.ms / 1000).toFixed(3)}`]);
    lines.push(`file '${frames.at(-1).name}'`);
    await writeFile(join(plan.framesDir, "frames.txt"), lines.join("\n") + "\n");
    console.log(`${frames.length} quadros em ${plan.framesDir}`);
  }
  ws.close();
} finally {
  edge.kill();
}
