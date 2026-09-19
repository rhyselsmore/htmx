const vm = require('node:vm');
const assert = require('node:assert/strict');
const { extractFunction } = require('./client-fixture.cjs');

// Execute the pinned parsing functions with only their required collaborators.
// These checks explain validation rules in the Go API; browser specs separately
// verify that real XHR, event dispatch, and swaps produce the expected result.
const names = [
  'handleTriggerHeader',
  'determineHistoryUpdates',
  'getSwapSpecification',
  'parseInterval',
  'formDataFromObject',
  'mergeObjects',
];
const selected = names.map(extractFunction).join('\n');
let delivered = [];
const ctx = vm.createContext({
  parseJSON: JSON.parse,
  isRawObject: (x) => Object.prototype.toString.call(x) === '[object Object]',
  triggerEvent: (target, name, detail) => delivered.push({ target, name, detail }),
  hasHeader: (xhr, pattern) => Object.keys(xhr.headers).some((k) => pattern.test(k + ':')),
  getClosestAttributeValue: (elt, name) => elt.attrs?.[name] || null,
  getInternalData: (elt) => elt,
  isAnchorLink: () => false,
  splitOnWhitespace: (value) => value.split(/\s+/),
  logError: (error) => {
    throw Error(error);
  },
  htmx: {
    config: {
      defaultSwapStyle: 'innerHTML',
      defaultSwapDelay: 0,
      defaultSettleDelay: 20,
    },
  },
  FormData,
  Blob,
});
vm.runInContext(selected, ctx);
const xhr = (headers) => ({
  headers,
  getResponseHeader: (name) => headers[name] ?? null,
});
const history = (elt, etc, headers = {}) =>
  // Strip VM prototypes before deep comparison; these results contain JSON data.
  JSON.parse(
    JSON.stringify(
      ctx.determineHistoryUpdates(elt, {
        xhr: xhr(headers),
        etc,
        pathInfo: { finalRequestPath: '/new', responsePath: '/new' },
      }),
    ),
  );
const checks = [];
function check(name, fn) {
  fn();
  checks.push(name);
}
check('Either standalone false history header suppresses the entire update', () => {
  for (const name of ['HX-Push-Url', 'HX-Replace-Url']) {
    assert.deepEqual(history({ boosted: true }, { replace: '/other' }, { [name]: 'false' }), {});
  }
});

check('Location suppression falls through to boosted push', () => {
  assert.deepEqual(history({ boosted: true }, { push: 'false', replace: 'false' }), {
    type: 'push',
    path: '/new',
  });
  assert.deepEqual(history({ boosted: false }, { push: 'false', replace: 'false' }), {});
});

check('Location replacement must suppress the implicit push', () => {
  assert.deepEqual(history({}, { push: 'true', replace: '/replacement' }), {
    type: 'push',
    path: '/new',
  });
  assert.deepEqual(history({}, { push: 'false', replace: '/replacement' }), {
    type: 'replace',
    path: '/replacement',
  });
});
// htmx 2.0.10 reuses the target variable across a trigger object. The encoder
// therefore emits untargeted events first, before any targeted event changes it.
check('Mixed trigger order leaks target to a later untargeted event', () => {
  delivered = [];
  ctx.handleTriggerHeader(
    xhr({ 'HX-Trigger': '{"a":{"target":"#other"},"z":null}' }),
    'HX-Trigger',
    '#original',
  );
  assert.deepEqual(
    delivered.map((x) => x.target),
    ['#other', '#other'],
  );
});

check('Untargeted-first ordering preserves each event target', () => {
  delivered = [];
  ctx.handleTriggerHeader(
    xhr({
      'HX-Trigger': '{"z":null,"a":{"target":"#other"},"b":{"target":"#third"}}',
    }),
    'HX-Trigger',
    '#original',
  );
  assert.deepEqual(
    delivered.map((x) => x.target),
    ['#original', '#other', '#third'],
  );
});

check('hasOwnProperty event name crashes object-form dispatch', () => {
  assert.throws(
    () =>
      ctx.handleTriggerHeader(
        xhr({ 'HX-Trigger': '{"hasOwnProperty":null}' }),
        'HX-Trigger',
        '#original',
      ),
    /hasOwnProperty/,
  );
});

check('Other previously banned names do not crash dispatch', () => {
  delivered = [];
  // Keep this as JSON: an object literal treats __proto__ specially and would
  // not exercise the same own property received from a response header.
  ctx.handleTriggerHeader(
    xhr({
      'HX-Trigger': '{"__proto__":null,"constructor":null,"prototype":null}',
    }),
    'HX-Trigger',
    '#original',
  );
  assert.equal(delivered.length, 3);
});

check('Swap parser supports selector colons, false, and explicit zero', () => {
  const value = ctx.getSwapSpecification(
    {},
    'innerHTML show:#item:hover:top focus-scroll:false swap:0ms',
  );
  assert.equal(value.showTarget, '#item:hover');
  assert.equal(value.show, 'top');
  assert.equal(value.focusScroll, false);
  assert.equal(value.swapDelay, 0);
});

check('Location values named hasOwnProperty crash form conversion', () => {
  assert.throws(() => ctx.formDataFromObject({ hasOwnProperty: 'value' }), /hasOwnProperty/);
});

check('Arrays of object location values silently stringify as Object', () => {
  assert.deepEqual(
    [...ctx.formDataFromObject({ item: [{ id: 1 }] }).entries()],
    [['item', '[object Object]']],
  );
});

check('UTF8 JSON passed through byte-string header decoding corrupts text', () => {
  // XHR exposes header bytes as a byte string. ASCII JSON escapes preserve the
  // Unicode payload through that boundary; raw UTF-8 header bytes do not.
  const bytes = Buffer.from(JSON.stringify({ message: 'café 😀' }), 'utf8');
  assert.notEqual(JSON.parse(bytes.toString('latin1')).message, 'café 😀');
  const ascii = '{"message":"caf\\u00e9 \\ud83d\\ude00"}';
  assert.equal(JSON.parse(Buffer.from(ascii).toString('latin1')).message, 'café 😀');
});

check('Location header objects need protected property names', () => {
  assert.throws(() => ctx.mergeObjects({}, { hasOwnProperty: 'x' }), /hasOwnProperty/);
  const unsafe = ctx.mergeObjects({}, JSON.parse('{"__proto__":"x"}'));
  assert.equal(Object.hasOwn(unsafe, '__proto__'), false);
  const safe = ctx.mergeObjects({}, { Hasownproperty: 'x' });
  assert.equal(safe.Hasownproperty, 'x');
});

console.log(JSON.stringify({ client: '2.0.10', checks: checks.length, passed: checks }, null, 2));
