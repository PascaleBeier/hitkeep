import path from "node:path";
import { defineConfig } from "vitest/config";

const cacheRoot = process.env.HITKEEP_FRONTEND_CACHE_DIR ?? ".angular/cache";
const cacheDir = path.join(cacheRoot, "vitest");

export default defineConfig({
    cacheDir,
    plugins: [
        {
            name: "hitkeep:workspace-cache",
            config: () => ({ cacheDir })
        }
    ]
});
