import fs from "node:fs/promises";
import path from "node:path";
import { createHash } from "node:crypto";

export async function pathExists(filePath: string) {
  try {
    await fs.access(filePath);
    return true;
  } catch {
    return false;
  }
}

export async function readText(filePath: string) {
  return fs.readFile(filePath, "utf8");
}

export async function hashFile(filePath: string) {
  const buffer = await fs.readFile(filePath);
  return createHash("sha256").update(buffer).digest("hex");
}

export async function walkFiles(root: string, extensions: string[]) {
  const found: string[] = [];

  async function walk(current: string) {
    const entries = await fs.readdir(current, { withFileTypes: true });
    for (const entry of entries) {
      const fullPath = path.join(current, entry.name);
      if (entry.isDirectory()) {
        if (entry.name === "node_modules" || entry.name === ".nuxt" || entry.name === ".output" || entry.name === "dist") {
          continue;
        }
        await walk(fullPath);
        continue;
      }

      if (entry.isFile() && extensions.includes(path.extname(entry.name))) {
        found.push(fullPath);
      }
    }
  }

  await walk(root);
  return found.sort();
}
