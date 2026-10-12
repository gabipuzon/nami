import { readFile, mkdir, copyFile } from 'node:fs/promises';
const pkg = JSON.parse(await readFile(new URL('./node_modules/typescript/package.json', import.meta.url), 'utf8'));
if (pkg.version !== '6.0.3') throw new Error('Expected TypeScript 6.0.3');
const target = new URL('../assets/', import.meta.url);
await mkdir(target, { recursive: true });
for (const [source, name] of [['lib/typescript.js', 'typescript.cjs'], ['LICENSE.txt', 'LICENSE.txt'], ['ThirdPartyNoticeText.txt', 'ThirdPartyNoticeText.txt']]) {
  await copyFile(new URL(`./node_modules/typescript/${source}`, import.meta.url), new URL(name, target));
}
