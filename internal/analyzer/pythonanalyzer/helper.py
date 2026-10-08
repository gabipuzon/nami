import ast
import hashlib
import io
import json
import os
import sys
import tokenize

request = json.load(sys.stdin)
files = []
for path in sorted(request['files']):
    try:
        with open(os.path.join(request['root'], path), 'rb') as source:
            raw = source.read()
        encoding, _ = tokenize.detect_encoding(io.BytesIO(raw).readline)
        content = raw.decode(encoding)
        tree = ast.parse(content, filename=path)
        file_hash = hashlib.sha256(raw).hexdigest()
        lines = content.splitlines(keepends=True)
        def editor_column(line, offset):
            prefix = lines[line-1].encode('utf-8')[:offset].decode('utf-8')
            return len(prefix.encode('utf-16-le')) // 2 + 1
        def location(node):
            snippet = ast.get_source_segment(content, node) or ''
            return dict(column=editor_column(node.lineno, node.col_offset), end_line=node.end_lineno,
                        end_column=editor_column(node.end_lineno, node.end_col_offset), snippet=snippet[:8192],
                        truncated=len(snippet)>8192, hash=file_hash)
        declarations = []
        for node in tree.body:
            if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
                declarations.append(dict(kind='FUNCTION', name=node.name, line=node.lineno))
            elif isinstance(node, ast.ClassDef):
                declarations.append(dict(kind='CLASS', name=node.name, line=node.lineno))
                for method in node.body:
                    if isinstance(method, (ast.FunctionDef, ast.AsyncFunctionDef)):
                        declarations.append(dict(kind='METHOD', name=node.name+'.'+method.name, line=method.lineno))
        imports = []
        # Imports in functions and conditional branches are source facts too.
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                for alias in node.names:
                    imports.append(dict(module=alias.name, level=0, name='', line=node.lineno, **location(node)))
            elif isinstance(node, ast.ImportFrom):
                if node.module:
                    imports.append(dict(module=node.module, level=node.level, name='', line=node.lineno, **location(node)))
                else:
                    for alias in node.names:
                        imports.append(dict(module='', level=node.level, name=alias.name, line=node.lineno, **location(node)))
        files.append(dict(path=path, declarations=sorted(declarations, key=lambda d:(d['kind'],d['name'],d['line'])), imports=sorted(imports,key=lambda i:(i['level'],i['module'],i['name'],i['line'])), error=''))
    except (OSError, SyntaxError, UnicodeError, ValueError) as error:
        reason = str(error)
        if isinstance(error, SyntaxError):
            reason = '%s at line %s, column %s' % (error.msg, error.lineno, error.offset)
        files.append(dict(path=path, declarations=[], imports=[], error=reason))
json.dump(dict(files=files, stdlib=sorted(getattr(sys, 'stdlib_module_names', sys.builtin_module_names))),sys.stdout,sort_keys=True)
