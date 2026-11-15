import { cpSync, mkdirSync, readdirSync, rmSync } from 'node:fs';
import { resolve, join } from 'node:path';

const destination = resolve('desktop/ui');
mkdirSync(destination, { recursive: true });
for (const file of readdirSync(destination)) {
  if (file.endsWith('.js') || file.endsWith('.css') || file === 'index.html') rmSync(join(destination, file));
}
cpSync(resolve('dist/campus/browser'), destination, { recursive: true });
