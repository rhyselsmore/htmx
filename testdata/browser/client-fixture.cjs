const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { readFileSync } = require('node:fs');
const path = require('node:path');

const client = readFileSync(path.join(__dirname, 'vendor/htmx-2.0.10.js'));
assert.equal(
  createHash('sha256').update(client).digest('hex'),
  '739498204ed3d437a37fd5ca3d16b1bda14e3b93353ea7440c7f129bd0bf93d5',
  'The vendored htmx client must match the pinned source',
);
const source = client.toString('utf8');

// Top-level functions in this pinned source have two-space indentation.
// These isolated probes supplement the browser tests; they do not emulate a DOM.
function extractFunction(name) {
  const start = source.indexOf(`  function ${name}(`);
  assert(start >= 0, `Missing client function: ${name}`);
  const end = source.indexOf('\n  }', start);
  assert(end > start, `Missing closing brace for client function: ${name}`);
  return source.slice(start, end + 4);
}

module.exports = { client, source, extractFunction };
