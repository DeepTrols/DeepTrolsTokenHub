import path from "node:path";
import { fileURLToPath } from "node:url";

const currentFile = fileURLToPath(import.meta.url);

export const harnessRoot = path.resolve(path.dirname(currentFile), "../..");
export const projectRoot = path.resolve(harnessRoot, "..");
export const aiRoot = path.join(projectRoot, "ai-nuxt");
export const backendRoot = projectRoot;
export const backendWebRoot = path.join(backendRoot, "web");
export const reportsRoot = path.join(harnessRoot, "reports");
