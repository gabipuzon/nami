'use strict';
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { builtinModules } = require('node:module');
const ts = require('./typescript.cjs');
if (ts.version !== '6.0.3') throw new Error('Unverified compiler version');
const input = JSON.parse(fs.readFileSync(0, 'utf8'));
const root = fs.realpathSync(input.root);
function read(rel) {
  const full = fs.realpathSync(path.resolve(root, rel));
  const relative = path.relative(root, full);
  if (relative === '..' || relative.startsWith('..' + path.sep) || path.isAbsolute(relative)) throw new Error('metadata/source outside repository');
  return fs.readFileSync(full, 'utf8');
}
const metadata = new Map();
function data(rel, config) {
  if (!metadata.has(rel)) {
    try {
      const text = read(rel);
      const result = config ? ts.parseConfigFileTextToJson(rel, text) : { config: JSON.parse(text) };
      if (result.error) throw new Error(ts.flattenDiagnosticMessageText(result.error.messageText, '\n'));
      metadata.set(rel, { path: rel, value: result.config });
    } catch (err) {
      metadata.set(rel, { path: rel, error: String(err.message) });
    }
  }
  return metadata.get(rel);
}
function nearest(rel, names, config) {
  for (let dir = path.posix.dirname(rel); ; dir = path.posix.dirname(dir)) {
    for (const name of names) {
      const candidate = path.posix.join(dir, name);
      if (fs.existsSync(path.resolve(root, candidate))) return data(candidate, config);
    }
    if (dir === '.') return null;
  }
}
const files = input.files.map(rel => {
  try {
    const text = read(rel);
    const source = ts.createSourceFile(rel, text, ts.ScriptTarget.Latest, true);
    if (source.parseDiagnostics.length) throw new Error(source.parseDiagnostics.map(d => ts.flattenDiagnosticMessageText(d.messageText, '\n')).join('; '));
    const declarations = [], imports = [], limitations = [];
    const named = n => n && ts.isComputedPropertyName(n) ? (ts.isStringLiteral(n.expression) || ts.isNumericLiteral(n.expression) ? n.expression.text : null) : n && (ts.isIdentifier(n) || ts.isStringLiteral(n) || ts.isNumericLiteral(n) || ts.isPrivateIdentifier(n)) ? n.text : null;
    const declaration = (kind, name, n) => { if (name) declarations.push({ kind, name }); };
    const binding = n => {
      if (ts.isIdentifier(n)) return [n.text];
      return n.elements.flatMap(e => ts.isOmittedExpression(e) ? [] : binding(e.name));
    };
    for (const n of source.statements) {
      if (ts.isFunctionDeclaration(n)) declaration('FUNCTION', named(n.name), n);
      else if (ts.isClassDeclaration(n)) {
        const name = named(n.name);
        declaration('CLASS', name, n);
        if (name) for (const m of n.members) {
          if (ts.isConstructorDeclaration(m)) declaration('METHOD', `${name}.constructor`, m);
          else if (ts.isMethodDeclaration(m)) {
            const method = named(m.name);
            if (method) declaration('METHOD', `${name}.${method}`, m);
          }
        }
      } else if (ts.isInterfaceDeclaration(n)) declaration('INTERFACE', named(n.name), n);
      else if (ts.isTypeAliasDeclaration(n)) declaration('TYPE', named(n.name), n);
      else if (ts.isEnumDeclaration(n)) declaration('ENUM', named(n.name), n);
      else if (ts.isVariableStatement(n)) for (const d of n.declarationList.declarations) {
        const callable = ts.isIdentifier(d.name) && d.initializer && (ts.isArrowFunction(d.initializer) || ts.isFunctionExpression(d.initializer));
        for (const name of binding(d.name)) declaration(callable ? 'FUNCTION' : 'VARIABLE', name, d);
      }
      if ((ts.isImportDeclaration(n) || ts.isExportDeclaration(n)) && n.moduleSpecifier && ts.isStringLiteral(n.moduleSpecifier)) {
        const start = n.getStart(source), end = n.getEnd();
        const a = source.getLineAndCharacterOfPosition(start), b = source.getLineAndCharacterOfPosition(end);
        const original = text.slice(start, end);
        imports.push({ specifier: n.moduleSpecifier.text, line: a.line + 1, column: a.character + 1, end_line: b.line + 1, end_column: b.character + 1,
          snippet: original.slice(0, 4096), hash: crypto.createHash('sha256').update(text).digest('hex'), truncated: original.length > 4096 });
      }
      if (ts.isImportEqualsDeclaration(n)) limitations.push('import-equals dependencies are unsupported');
    }
    function visit(n) {
      if (ts.isCallExpression(n) && (n.expression.kind === ts.SyntaxKind.ImportKeyword || ts.isIdentifier(n.expression) && n.expression.text === 'require')) limitations.push('dynamic import/require call dependencies are unsupported; binding not verified');
      ts.forEachChild(n, visit);
    }
    visit(source);
    return { path: rel, declarations, imports, limitations: [...new Set(limitations)], config: nearest(rel, ['tsconfig.json', 'jsconfig.json'], true), package: nearest(rel, ['package.json'], false) };
  } catch (err) { return { path: rel, error: String(err.message) }; }
});
process.stdout.write(JSON.stringify({ files, stdlib: builtinModules }));
