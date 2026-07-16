import { access, cp, mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const sourceDir = resolve(repositoryRoot, 'web/dist');
const targetDir = resolve(repositoryRoot, 'internal/panel/static');
const preservedFiles = new Set();

await Promise.all([
  access(resolve(sourceDir, 'index.html')),
  access(resolve(sourceDir, 'login.html')),
]);

await mkdir(targetDir, { recursive: true });
for (const entry of await readdir(targetDir, { withFileTypes: true })) {
  if (!preservedFiles.has(entry.name)) {
    await rm(resolve(targetDir, entry.name), { recursive: true, force: true });
  }
}

await cp(sourceDir, targetDir, { recursive: true, force: true });

for (const entry of ['index.html', 'login.html']) {
  const target = resolve(targetDir, entry);
  const content = await readFile(target, 'utf8');
  await writeFile(target, content.replace(/\r\n/g, '\n'), 'utf8');
}
