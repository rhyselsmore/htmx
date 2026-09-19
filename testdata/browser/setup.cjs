const { spawn, execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

// The Go fixture binds an available port and prints its origin once it is ready.
// Remove startup listeners once settled so normal teardown is not a startup error.
function waitForOrigin(child) {
  return new Promise((resolve, reject) => {
    let output = '';
    const timeout = setTimeout(() => finish(new Error('Fixture startup timed out')), 10000);

    function finish(error, origin) {
      clearTimeout(timeout);
      child.off('error', onError);
      child.off('exit', onExit);
      child.stdout.off('data', onData);
      if (error) reject(error);
      else resolve(origin);
    }

    function onError(error) {
      finish(error);
    }

    function onExit(code, signal) {
      finish(new Error(`Fixture exited before startup: ${signal || code}`));
    }

    function onData(data) {
      output += data;
      const newline = output.indexOf('\n');
      if (newline !== -1) finish(null, output.slice(0, newline).trim());
    }

    child.once('error', onError);
    child.once('exit', onExit);
    child.stdout.on('data', onData);
  });
}

module.exports = async () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'htmx-browser-'));
  const binary = path.join(directory, 'server');
  const env = {
    ...process.env,
    GOCACHE: process.env.GOCACHE || path.join(os.tmpdir(), 'htmx-browser-go'),
  };
  let child;
  let closed;

  // Build a binary instead of using go run so teardown owns the actual server
  // process. The same cleanup handles build failure, startup failure, and success.
  async function cleanup() {
    try {
      if (child) {
        if (child.pid && child.exitCode === null && child.signalCode === null) {
          child.kill('SIGTERM');
        }
        // close also fires after spawn errors and waits for stdout to close.
        await closed;
      }
    } finally {
      fs.rmSync(directory, { recursive: true, force: true });
    }
  }

  try {
    execFileSync('go', ['build', '-o', binary, './server'], {
      cwd: __dirname,
      env,
      stdio: 'inherit',
    });
    child = spawn(binary, [], {
      cwd: __dirname,
      env,
      stdio: ['ignore', 'pipe', 'inherit'],
    });
    closed = new Promise((resolve) => child.once('close', resolve));
    // Playwright passes this environment value to each subsequently started worker.
    process.env.HTMX_TEST_BASE_URL = await waitForOrigin(child);
    return cleanup;
  } catch (error) {
    await cleanup();
    throw error;
  }
};
