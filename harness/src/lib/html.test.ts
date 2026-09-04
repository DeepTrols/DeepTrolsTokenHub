import assert from "node:assert/strict";
import { test } from "node:test";
import { pageText } from "./html.ts";

test("repository names in dev resources are not visible brand text", () => {
  const html = `<!doctype html><html><head>
    <title>智曜TokenHub</title>
    <link rel="modulepreload" href="/ai/_nuxt/DeepTrolsTokenHub/app.js">
    <script type="module" src="/ai/_nuxt/DeepTrolsTokenHub/app.js"></script>
    <script>window.config = { root: "DeepTrolsTokenHub" }</script>
    <style>.DeepTrols { color: red; }</style>
    </head><body><h1>智曜TokenHub</h1><!-- DeepTrols --></body></html>`;
  assert.doesNotMatch(pageText(html), /DeepTrols/i);
  assert.match(pageText(html), /智曜TokenHub/);
});

test("legacy title and visible content are still detected", () => {
  assert.match(pageText("<title>DeepTrols</title><h1>智曜TokenHub</h1>"), /DeepTrols/);
  assert.match(pageText("<h1>智曜算力超市</h1>"), /智曜算力超市/);
});

test("separate hero labels are not concatenated into an old brand name", () => {
  assert.equal(pageText("<h1><span>智曜</span><span>算力超市</span></h1>"), "智曜 算力超市");
});

test("HTML entities are decoded before checking branding", () => {
  assert.match(pageText("<p>Deep&#84;rols</p>"), /DeepTrols/);
  assert.equal(pageText("<h1>智曜TokenHub &amp; AI</h1>"), "智曜TokenHub & AI");
});
