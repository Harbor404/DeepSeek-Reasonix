import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const home = await readFile(new URL("../pages/index.astro", import.meta.url), "utf8");
const docs = await readFile(new URL("../pages/docs.astro", import.meta.url), "utf8");

const tag = (html, id) => html.match(new RegExp(`<button[^>]*id="${id}"[^>]*>`))?.[0] ?? "";
const pane = (id) => home.match(new RegExp(`<div class="([^"]*)" id="${id}"[^>]*>`));

test("Studio is the first, selected and only visible download pane", () => {
  const firstTab = home.match(/<button class="dl-tab[^"]*" id="(download-tab-[a-z]+)"/)[1];
  assert.equal(firstTab, "download-tab-studio");
  assert.match(tag(home, "download-tab-studio"), /aria-selected="true"/);
  assert.match(pane("download-pane-studio")[1], /\bactive\b/);
  for (const id of ["desktop", "cli", "npm", "brew", "vscode"]) {
    assert.doesNotMatch(pane(`download-pane-${id}`)[1], /\bactive\b/);
    assert.match(tag(home, `download-tab-${id}`), /aria-selected="false"/);
  }
  assert.equal((home.match(/dl-pane active/g) ?? []).length, 1);
});

test("the hero primary action is Studio and 1.x is labelled as the command line", () => {
  const primary = home.match(/<a class="btn btn-dark" data-os-dl[^>]*>/)[0];
  assert.match(primary, /data-goto="studio"/);
  assert.doesNotMatch(home, /Download stable 1\.x/);
  assert.match(home, /Command line \(1\.x\)/);
  assert.match(home, /命令行（1\.x）/);
});

test("the Studio pane offers every build the release model attests", () => {
  for (const name of [
    "ReasonixStudio-darwin-arm64.dmg",
    "ReasonixStudio-darwin-amd64.dmg",
    "ReasonixStudio-windows-amd64-installer.exe",
    "ReasonixStudio-linux-amd64.deb",
  ]) {
    assert.match(home, new RegExp(`data-studio-asset="${name.replace(/\./g, "\\.")}"`));
    assert.match(docs, new RegExp(`data-studio-asset="${name.replace(/\./g, "\\.")}"`));
  }
  assert.doesNotMatch(home + docs, /dl\.reasonix\.io\/studio-v\d/);
});

test("the 1.x CLI stays reachable from the Studio pane", () => {
  const studio = home.slice(home.indexOf('id="download-pane-studio"'), home.indexOf('id="download-pane-desktop"'));
  assert.match(studio, /npm i -g reasonix/);
  assert.match(studio, /data-goto="cli"/);
});

test("docs lead the install section with Studio", () => {
  assert.ok(docs.indexOf('id="studio"') < docs.indexOf('id="install"'));
});

test("the hero label has no dangling 'for' without JavaScript", () => {
  const hero = home.match(/<a class="btn btn-dark" data-os-dl[\s\S]*?<\/a>/)[0];
  assert.match(hero, /class="os-for" hidden/g);
  assert.doesNotMatch(hero.replace(/<span class="os-for"[\s\S]*?<\/span><\/span>/g, ""), / for /);
});

test("copy does not promise shared sessions or one engine across 1.x and Studio", () => {
  assert.doesNotMatch(home, /waiting for you in the desktop app|one local engine|same local engine/i);
  assert.match(home, /do not share sessions/);
});

test("platform line names the shipped builds", () => {
  assert.match(home, /macOS \(Apple Silicon, Intel\) · Windows x64 · Linux amd64 \(\.deb\)/);
});

test("docs Studio cards use a 2x2 grid and the 1.x kicker keeps its case", () => {
  assert.match(docs, /desktop-downloads desktop-downloads--two/);
  assert.match(home, /<span class="keep-case">1\.x<\/span>/);
});
