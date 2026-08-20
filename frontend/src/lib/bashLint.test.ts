import assert from 'node:assert/strict';
import { basicBashLint } from './bashLint';

// A command pasted with a quote missing is the one that hangs a terminal waiting for input rather
// than failing, which is the whole reason this check exists.
{
  assert.deepEqual(basicBashLint("echo 'hello"), ['scripts.lint.singleQuote']);
  assert.deepEqual(basicBashLint('echo "hello'), ['scripts.lint.doubleQuote']);
  assert.deepEqual(basicBashLint("echo 'hello'"), []);
  assert.deepEqual(basicBashLint('echo "hello"'), []);
}

// A quote inside the other kind is text, not an opener.
{
  assert.deepEqual(basicBashLint(`echo "it's fine"`), [], 'an apostrophe inside double quotes');
  assert.deepEqual(basicBashLint(`echo 'say "hi"'`), [], 'a double quote inside single quotes');
}

// An escaped quote closes nothing.
{
  assert.deepEqual(basicBashLint('echo \\"'), [], 'an escaped double quote is not an opener');
  assert.deepEqual(basicBashLint('echo "a\\"b"'), [], 'an escaped quote inside a string');
}

// The heredoc opener follows a command; the regex this replaced was anchored to the start of the
// line and so never fired at all.
{
  assert.deepEqual(basicBashLint(['cat <<EOF', 'body', ''].join('\n')), ['scripts.lint.heredoc']);
  assert.deepEqual(
    basicBashLint(['ssh host <<EOF', 'body', 'EOF'].join('\n')),
    [],
    'a closed heredoc after a command',
  );
  assert.deepEqual(
    basicBashLint(["cat <<'EOF'", 'body', 'EOF'].join('\n')),
    [],
    'a quoted heredoc delimiter',
  );
  assert.deepEqual(
    basicBashLint(['cat <<-EOF', 'body', 'EOF'].join('\n')),
    [],
    'a dash heredoc',
  );
}

// A herestring takes a word rather than opening a block, so it must not arm the check.
{
  assert.deepEqual(basicBashLint('grep x <<<"$var"'), [], 'a herestring is not a heredoc');
}

// Looking anywhere in the line is only safe because the quote state is known at that point.
{
  assert.deepEqual(basicBashLint('echo "a << b"'), [], 'a << inside quotes is text');
  assert.deepEqual(basicBashLint("echo 'a << b'"), [], 'and inside single quotes too');
}

// Shell syntax inside a heredoc body is text: an apostrophe in prose must not read as an opener.
{
  assert.deepEqual(
    basicBashLint(['cat <<EOF', "it's fine", 'EOF'].join('\n')),
    [],
    'an apostrophe inside a heredoc body is not an unterminated quote',
  );
}

// A quote spans newlines in the shell, so an unterminated one swallows what follows - including
// what would otherwise have opened a heredoc. Reporting both would claim two independent problems
// where there is one cause.
{
  assert.deepEqual(
    basicBashLint(["echo 'a", 'cat <<EOF', 'body', ''].join('\n')),
    ['scripts.lint.singleQuote'],
  );
}

// Two problems that really are independent: a heredoc that closed, and a quote that did not.
{
  assert.deepEqual(
    basicBashLint(['cat <<EOF', 'body', 'EOF', 'echo "b'].join('\n')),
    ['scripts.lint.doubleQuote'],
    'a closed heredoc leaves only the open quote',
  );
}

{
  assert.deepEqual(basicBashLint(''), [], 'nothing typed yet is not a problem');
}

console.log('OK bashLint');
