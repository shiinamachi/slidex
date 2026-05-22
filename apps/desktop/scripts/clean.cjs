const fs = require("node:fs");
const path = require("node:path");

for (const dir of ["dist", "release"]) {
  fs.rmSync(path.join(__dirname, "..", dir), { recursive: true, force: true });
}
