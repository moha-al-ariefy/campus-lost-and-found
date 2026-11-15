import { existsSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const projectRoot = fileURLToPath(new URL('../', import.meta.url));
const localExecutable = fileURLToPath(new URL(`../tmp/tools/bin/wails${process.platform === 'win32' ? '.exe' : ''}`, import.meta.url));
const executable = existsSync(localExecutable) ? localExecutable : 'wails';
const result = spawnSync(executable, process.argv.slice(2), {
  cwd: `${projectRoot}/desktop`,
  stdio: 'inherit',
});
if (result.error) {
  console.error('Wails could not start. Install the version listed in README and add its bin directory to PATH.');
}
process.exit(result.status ?? 1);
