import { parse, type DefaultTreeAdapterMap } from "parse5";

export function pageText(html: string): string {
  const text: string[] = [];

  function visit(node: DefaultTreeAdapterMap["node"]) {
    // Dev-server resource URLs and hydration data are not page branding.
    if ("tagName" in node && ["script", "style", "template"].includes(node.tagName)) {
      return;
    }
    if ("value" in node) {
      text.push(node.value);
    }
    if ("childNodes" in node) {
      node.childNodes.forEach(visit);
    }
  }

  visit(parse(html));
  // Keep separately rendered labels separate instead of inventing a combined word.
  return text.join(" ");
}
