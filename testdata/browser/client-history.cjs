const vm = require('node:vm');
const assert = require('node:assert/strict');
const { source, extractFunction } = require('./client-fixture.cjs');

// Probe branches that are difficult to force through XHR, such as a missing
// responseURL. DOM mutation and actual navigation are covered by the specs.
// selectOOB lives inside swap(), so extract its block using the pinned markers.
const selectStart = source.indexOf('        // select-oob swaps');
const selectEnd = source.indexOf('        // oob swaps', selectStart);
assert(selectStart >= 0 && selectEnd > selectStart);
let requests = [];
let oob = [];
let dispatch = [];

// These stubs record the client's decisions, without implementing a DOM or
// issuing a follow-up request. Unexpected client errors must fail the probe.
const ctx = vm.createContext({
  URL,
  Element: class {},
  DUMMY_ELT: {},
  parseJSON: JSON.parse,
  triggerEvent: () => true,
  resolveTarget: (s) => (s ? { selector: s } : null),
  issueAjaxRequest: (verb, path, source, event, etc) =>
    requests.push({ verb, path, source, event, etc }),
  getClosestAttributeValue: (elt, name) => elt.attrs?.[name] || null,
  getInternalData: (elt) => elt,
  getDocument: () => ({ body: {} }),
  triggerErrorEvent: () => {
    throw Error('unexpected client error');
  },
  oobSwap: (style, elt) => {
    oob.push([elt.id, style]);
    elt.remove();
  },
  htmx: { config: { defaultSwapStyle: 'innerHTML' } },
  getExtensions: () => [],
  swapInnerHTML: () => dispatch.push('innerHTML'),
  swapOuterHTML: () => dispatch.push('outerHTML'),
  swapAfterBegin: () => dispatch.push('afterbegin'),
  swapBeforeBegin: () => dispatch.push('beforebegin'),
  swapBeforeEnd: () => dispatch.push('beforeend'),
  swapAfterEnd: () => dispatch.push('afterend'),
  swapDelete: () => dispatch.push('delete'),
});
vm.runInContext(
  [
    'hasHeader',
    'getPathFromResponse',
    'ajaxHelper',
    'handleAjaxResponse',
    'determineHistoryUpdates',
    'swapWithStyle',
  ]
    .map(extractFunction)
    .join('\n'),
  ctx,
);
vm.runInContext(
  'function selected(fragment,swapOptions,settleInfo,rootNode){\n' +
    source.slice(selectStart, selectEnd) +
    '\n}',
  ctx,
);
const xhr = (headers = {}, responseURL = 'https://example.test/final?q=1') => ({
  responseURL,
  getAllResponseHeaders: () =>
    Object.keys(headers)
      .map((k) => k + ': ' + headers[k])
      .join('\r\n'),
  getResponseHeader: (name) => headers[name] ?? null,
});
const history = (
  etc,
  headers = {},
  pathInfo = { finalRequestPath: '/start', responsePath: '/final?q=1' },
  elt = {},
) =>
  // VM objects have different prototypes; compare their JSON data in this realm.
  JSON.parse(
    JSON.stringify(ctx.determineHistoryUpdates(elt, { xhr: xhr(headers), etc, pathInfo })),
  );
const checks = [];
function check(name, fn) {
  fn();
  checks.push(name);
}
function select(value, ids = ['alerts', 'counter']) {
  oob = [];
  const found = new Map();
  ids.forEach((id) => found.set('#' + id, { id, remove: () => found.delete('#' + id) }));
  // An OOB swap consumes its source node. Keeping that side effect in the stub
  // lets duplicate selections and source ordering behave like the real client.
  ctx.selected(
    {
      querySelector: (selector) => found.get(selector),
    },
    { selectOOB: value },
    {},
    {},
  );
  return { oob: [...oob] };
}

// HX-Location starts another request. History options must survive forwarding
// and then be interpreted using that request's final URL, including redirects.
check('HX-Location forwards selectOOB and the complete replacement pair', () => {
  requests = [];
  ctx.handleAjaxResponse(
    {},
    {
      xhr: xhr({
        'HX-Location':
          '{"path":"/start","selectOOB":"#alerts:outerHTML,#counter:innerHTML","push":"false","replace":"true"}',
      }),
      etc: {},
    },
  );
  assert.equal(requests.length, 1);
  assert.equal(requests[0].verb, 'get');
  assert.equal(requests[0].path, '/start');
  assert.equal(requests[0].etc.selectOOB, '#alerts:outerHTML,#counter:innerHTML');
  assert.equal(requests[0].etc.push, 'false');
  assert.equal(requests[0].etc.replace, 'true');
});

check('Omitting push suppression defeats destination replacement', () => {
  requests = [];
  ctx.handleAjaxResponse(
    {},
    {
      xhr: xhr({ 'HX-Location': '{"path":"/start","replace":"true"}' }),
      etc: {},
    },
  );
  assert.deepEqual(history(requests[0].etc), {
    type: 'push',
    path: '/final?q=1',
  });
});

check('Destination replacement follows final response path and query', () => {
  const responsePath = ctx.getPathFromResponse(
    xhr({}, 'https://example.test/redirected?result=ok'),
  );
  assert.equal(responsePath, '/redirected?result=ok');
  assert.deepEqual(
    history({ push: 'false', replace: 'true' }, {}, { finalRequestPath: '/start', responsePath }),
    { type: 'replace', path: '/redirected?result=ok' },
  );
});

check('Missing response URL uses final request path and retains request anchor', () => {
  assert.equal(ctx.getPathFromResponse(xhr({}, '')), undefined);
  assert.deepEqual(
    history(
      { push: 'false', replace: 'true' },
      {},
      {
        finalRequestPath: '/start?q=2',
        responsePath: null,
        anchor: 'details',
      },
    ),
    { type: 'replace', path: '/start?q=2#details' },
  );
});

check('Literal true and boolean true have different semantics', () => {
  assert.deepEqual(history({ push: 'false', replace: true }), {
    type: 'replace',
    path: true,
  });
  assert.throws(
    () =>
      history(
        { push: 'false', replace: true },
        {},
        {
          finalRequestPath: '/start',
          responsePath: '/final',
          anchor: 'details',
        },
      ),
    /indexOf/,
  );
  assert.deepEqual(history({ push: 'false', replace: '/true' }), {
    type: 'replace',
    path: '/true',
  });
});

check('Follow-up response headers override destination replacement', () => {
  assert.deepEqual(history({ push: 'false', replace: 'true' }, { 'HX-Push-Url': '/override' }), {
    type: 'push',
    path: '/override',
  });
  assert.deepEqual(history({ push: 'false', replace: 'true' }, { 'HX-Push-Url': 'false' }), {});
});

check('Explicit destination replacement wins over inherited history and boosting', () => {
  assert.deepEqual(
    history({ push: 'false', replace: 'true' }, {}, undefined, {
      boosted: true,
      attrs: { 'hx-push-url': '/inherited', 'hx-replace-url': '/other' },
    }),
    { type: 'replace', path: '/final?q=1' },
  );
});
// This syntax is an ID list with optional swap styles, not a general selector
// list. Record the client's permissive parsing so package validation stays honest.
check('selectOOB accepts default and explicit styles in source order', () => {
  assert.deepEqual(select('#counter:innerHTML,#alerts').oob, [
    ['counter', 'innerHTML'],
    ['alerts', 'true'],
  ]);
  assert.deepEqual(select(' alerts:true ').oob, [['alerts', 'true ']]);
});

check('selectOOB strips one leading hash and silently skips missing source IDs', () => {
  assert.deepEqual(select('missing:outerHTML,alerts:beforeend').oob, [['alerts', 'beforeend']]);
});

check('Duplicate OOB source selection is consumed only on its first occurrence', () => {
  assert.deepEqual(select('#alerts:outerHTML,#alerts:innerHTML').oob, [['alerts', 'outerHTML']]);
});

check('Extra colons truncate rather than describing an OOB target selector', () => {
  assert.deepEqual(select('#alerts:beforeend:#elsewhere').oob, [['alerts', 'beforeend']]);
});

check('OOB textContent falls back to the default HTML swap', () => {
  dispatch = [];
  ctx.swapWithStyle('textContent', {}, {}, {}, {});
  assert.deepEqual(dispatch, ['innerHTML']);
});

check('All eight intended OOB strategies dispatch without fallback', () => {
  for (const style of [
    'innerHTML',
    'outerHTML',
    'beforebegin',
    'afterbegin',
    'beforeend',
    'afterend',
    'delete',
    'none',
  ]) {
    dispatch = [];
    ctx.swapWithStyle(style, {}, {}, {}, {});
    assert.deepEqual(dispatch, style === 'none' ? [] : [style]);
  }
});

console.log(JSON.stringify({ client: '2.0.10', checks: checks.length, passed: checks }, null, 2));
