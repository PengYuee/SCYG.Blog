import { mkdtemp, readFile, readdir, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { spawnSync } from "node:child_process"
const temporary = await mkdtemp(join(tmpdir(), "scyg-api-"))
try {
  const generated = spawnSync(process.execPath, ["node_modules/@hey-api/openapi-ts/bin/run.js"], { stdio: "inherit", env: { ...process.env, SCYG_API_OUTPUT: temporary } })
  if (generated.error) throw generated.error
  if (generated.status !== 0) process.exitCode = generated.status ?? 1
  else {
    const expected = (await readdir(temporary)).sort()
    const actual = (await readdir("src/request/generated")).sort()
    if (JSON.stringify(expected) !== JSON.stringify(actual)) throw new Error("API generated file set differs from the workspace")
    for (const name of expected) {
      const [left, right] = await Promise.all([readFile(join(temporary, name)), readFile(join("src/request/generated", name))])
      if (!left.equals(right)) throw new Error(`API definition drift: ${name}; run pnpm generate:api`)
    }
  }
} finally { await rm(temporary, { recursive: true, force: true }) }
