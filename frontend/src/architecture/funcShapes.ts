// Function shapes for the frontend budgets, measured on the real TypeScript
// AST rather than by regex.
//
// Only named functions are measured: declarations, class methods, and arrow or
// function expressions bound to a variable. An inline callback has no stable
// name to record a baseline against, and renaming its enclosing statement
// would silently drop the entry. Bulk hidden inside callbacks is still caught
// by the file budget.

import ts from 'typescript';
import { blankComments, scriptBlockRe, svelteScript } from './measure';

export interface FuncShape {
  codeLines: number;
  params: number;
  nesting: number;
}

/**
 * Blanks everything outside <script> blocks so the result parses as
 * TypeScript while every line number still points at the original component
 * line. Extracting the script text instead would shift every position by the
 * size of the markup above it.
 */
export function svelteAsTypeScript(src: string): string {
  if (svelteScript(src) === '') return '';
  const blank = (text: string): string => text.replace(/[^\n]/g, ' ');

  let out = '';
  let index = 0;
  const re = scriptBlockRe();
  let match: RegExpExecArray | null;
  while ((match = re.exec(src))) {
    const scriptStart = match.index + match[0].indexOf('>') + 1;
    out += blank(src.slice(index, scriptStart));
    out += match[1];
    index = scriptStart + match[1].length;
  }
  return out + blank(src.slice(index));
}

export function measureFuncs(relPath: string, src: string): Map<string, FuncShape> {
  const text = relPath.endsWith('.svelte') ? svelteAsTypeScript(src) : src;
  const out = new Map<string, FuncShape>();
  if (text.trim() === '') return out;

  const sourceFile = ts.createSourceFile(relPath, text, ts.ScriptTarget.ES2020, true);
  const codeLines = new Set<number>();
  blankComments(text)
    .split('\n')
    .forEach((line, i) => {
      if (line.trim() !== '') codeLines.add(i + 1);
    });

  const lineOf = (pos: number): number => sourceFile.getLineAndCharacterOfPosition(pos).line + 1;

  const visit = (node: ts.Node): void => {
    const named = namedFunction(node);
    if (named) {
      out.set(`${relPath}::${named.name}`, {
        codeLines: bodyCodeLines(named.fn, lineOf, codeLines),
        params: named.fn.parameters.length,
        nesting: maxNesting(named.fn.body),
      });
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFile);
  return out;
}

interface NamedFunction {
  name: string;
  fn: ts.FunctionLikeDeclaration;
}

function namedFunction(node: ts.Node): NamedFunction | null {
  if (ts.isFunctionDeclaration(node) && node.name && node.body) {
    return { name: node.name.text, fn: node };
  }
  if (ts.isMethodDeclaration(node) && node.body && ts.isIdentifier(node.name)) {
    return { name: node.name.text, fn: node };
  }
  if (
    ts.isVariableDeclaration(node) &&
    ts.isIdentifier(node.name) &&
    node.initializer &&
    (ts.isArrowFunction(node.initializer) || ts.isFunctionExpression(node.initializer)) &&
    node.initializer.body
  ) {
    return { name: node.name.text, fn: node.initializer };
  }
  return null;
}

function bodyCodeLines(
  fn: ts.FunctionLikeDeclaration,
  lineOf: (pos: number) => number,
  codeLines: Set<number>
): number {
  if (!fn.body) return 0;
  // A concise arrow body (`x => x + 1`) has no braces to count between.
  if (!ts.isBlock(fn.body)) return 1;

  const first = lineOf(fn.body.getStart()) + 1;
  const last = lineOf(fn.body.getEnd());
  let count = 0;
  for (let line = first; line < last; line++) {
    if (codeLines.has(line)) count++;
  }
  return count;
}

/**
 * Deepest chain of nested control structures, matching maxNesting in
 * test/unit/architecture: a switch counts once rather than once per case, and
 * an `else if` continues its chain at the depth of the `if` it extends. Both
 * read as a list of alternatives, not as a staircase.
 */
export function maxNesting(body: ts.Node | undefined): number {
  if (!body) return 0;
  let deepest = 0;
  const record = (depth: number): void => {
    if (depth + 1 > deepest) deepest = depth + 1;
  };

  const walk = (node: ts.Node, depth: number): void => {
    if (ts.isIfStatement(node)) {
      record(depth);
      walk(node.thenStatement, depth + 1);
      if (node.elseStatement) {
        // else-if extends the chain; a plain else opens a new level.
        walk(node.elseStatement, ts.isIfStatement(node.elseStatement) ? depth : depth + 1);
      }
      // A function expression can hide in the condition itself.
      walk(node.expression, depth);
      return;
    }
    if (isLoop(node) || ts.isSwitchStatement(node) || ts.isTryStatement(node)) {
      record(depth);
      ts.forEachChild(node, (child) => walk(child, depth + 1));
      return;
    }
    ts.forEachChild(node, (child) => walk(child, depth));
  };

  const isLoop = (node: ts.Node): boolean =>
    ts.isForStatement(node) ||
    ts.isForInStatement(node) ||
    ts.isForOfStatement(node) ||
    ts.isWhileStatement(node) ||
    ts.isDoStatement(node);

  ts.forEachChild(body, (child) => walk(child, 0));
  return deepest;
}
