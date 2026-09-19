const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const fs = require('node:fs');
const path = require('node:path');
const { PassThrough } = require('node:stream');
const test = require('node:test');
const vm = require('node:vm');

// Exercise setup's process boundaries without compiling Go or launching a server.
// In particular, Node emits error + close (without exit) when spawn fails.
function fixtureProcess() {
  const child = new EventEmitter();
  child.stdout = new PassThrough();
  child.pid = 123;
  child.exitCode = null;
  child.signalCode = null;
  child.kill = (signal) => {
    child.signalCode = signal;
    process.nextTick(() => {
      child.emit('exit', null, signal);
      child.stdout.end();
      child.emit('close', null, signal);
    });
    return true;
  };
  return child;
}

function loadSetup({ start, buildError } = {}) {
  const child = fixtureProcess();
  const env = {};
  let directory;
  const childProcess = {
    execFileSync(command, args) {
      directory = path.dirname(args[2]);
      assert.ok(fs.existsSync(directory));
      if (buildError) throw buildError;
    },
    spawn() {
      process.nextTick(() => start(child));
      return child;
    },
  };
  const context = {
    module: { exports: {} },
    __dirname,
    process: { env },
    setTimeout,
    clearTimeout,
    require: (name) => (name === 'node:child_process' ? childProcess : require(name)),
  };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'setup.cjs'), 'utf8'), context);
  return {
    setup: context.module.exports,
    child,
    env,
    assertRemoved: () => assert.equal(fs.existsSync(directory), false),
  };
}

test('startup accepts a split origin line and teardown removes the binary directory', async () => {
  const fixture = loadSetup({
    start(child) {
      child.stdout.write('http://127.0.0.1:');
      child.stdout.write('12345\n');
    },
  });
  const teardown = await fixture.setup();
  try {
    assert.equal(fixture.env.HTMX_TEST_BASE_URL, 'http://127.0.0.1:12345');
    assert.equal(fixture.child.stdout.listenerCount('data'), 0);
  } finally {
    await teardown();
  }
  assert.equal(fixture.child.signalCode, 'SIGTERM');
  fixture.assertRemoved();
});

test('build failure removes the temporary directory', async () => {
  const error = new Error('build failed');
  const fixture = loadSetup({ buildError: error });
  await assert.rejects(fixture.setup, (actual) => actual === error);
  fixture.assertRemoved();
});

test('spawn failure cleans up without waiting for an exit event', { timeout: 1000 }, async () => {
  const error = new Error('spawn failed');
  const fixture = loadSetup({
    start(child) {
      child.pid = undefined;
      child.emit('error', error);
      child.emit('close', -2, null);
    },
  });
  await assert.rejects(fixture.setup, (actual) => actual === error);
  fixture.assertRemoved();
});

test('early signal exit reports the signal and cleans up', { timeout: 1000 }, async () => {
  const fixture = loadSetup({ start: (child) => child.kill('SIGTERM') });
  await assert.rejects(fixture.setup, /Fixture exited before startup: SIGTERM/);
  fixture.assertRemoved();
});
