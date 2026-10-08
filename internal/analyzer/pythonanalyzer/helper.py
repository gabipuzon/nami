import ast
import json
import os
import sys
import tokenize

request = json.load(sys.stdin)
files = []
for path in sorted(request['files']):
    try:
        with tokenize.open(os.path.join(request['root'], path)) as source:
            tree = ast.parse(source.read(), filename=path)
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
                    imports.append(dict(module=alias.name, level=0, name='', line=node.lineno))
            elif isinstance(node, ast.ImportFrom):
                if node.module:
                    imports.append(dict(module=node.module, level=node.level, name='', line=node.lineno))
                else:
                    for alias in node.names:
                        imports.append(dict(module='', level=node.level, name=alias.name, line=node.lineno))
        files.append(dict(path=path, declarations=sorted(declarations, key=lambda d:(d['kind'],d['name'],d['line'])), imports=sorted(imports,key=lambda i:(i['level'],i['module'],i['name'],i['line'])), error=''))
    except (OSError, SyntaxError, UnicodeError, ValueError) as error:
        reason = str(error)
        if isinstance(error, SyntaxError):
            reason = '%s at line %s, column %s' % (error.msg, error.lineno, error.offset)
        files.append(dict(path=path, declarations=[], imports=[], error=reason))
json.dump(dict(files=files, stdlib=sorted(getattr(sys, 'stdlib_module_names', sys.builtin_module_names))),sys.stdout,sort_keys=True)
