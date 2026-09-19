const { test, expect } = require('@playwright/test');
const http = require('node:http');
const { client } = require('./client-fixture.cjs');

// Handwritten responses establish the client's behavior independently of the
// Go encoder. package.spec.cjs tests the same wire protocol through the package.

let server;
let origin;

test.beforeAll(async () => {
  server = http.createServer((req, res) => {
    if (req.url === '/htmx.js') {
      res.setHeader('Content-Type', 'text/javascript');
      return res.end(client);
    }
    if (req.url === '/events') {
      // Keep the literal ASCII escapes: Node must send backslash-u sequences,
      // not raw Unicode bytes, for the XHR header decoding regression.
      res.setHeader(
        'HX-Trigger',
        '{"z-plain":{"value":"caf\\u00e9 \\ud83d\\ude00"},"a-target":{"target":"#alerts","value":7}}',
      );
      return res.end('updated');
    }
    if (req.url === '/shapes') {
      res.setHeader(
        'HX-Trigger',
        '{"object":{"count":3},"scalar":7,"array":[1,2],"nil":null,"targeted":{"target":"#alerts","value":[3,4]}}',
      );
      return res.end('shapes');
    }
    if (req.url === '/navigate') {
      // Both fields are required: replacement alone leaves the implicit push
      // enabled, which takes precedence in the pinned client's history logic.
      res.setHeader(
        'HX-Location',
        JSON.stringify({
          path: '/redirect',
          target: '#items',
          select: '#items',
          swap: 'outerHTML',
          selectOOB: '#alerts:outerHTML,#count:innerHTML',
          push: 'false',
          replace: 'true',
        }),
      );
      return res.end();
    }
    if (req.url === '/redirect') {
      res.writeHead(302, { Location: '/destination?q=1' });
      return res.end();
    }
    if (req.url === '/destination?q=1')
      return res.end(
        '<section id="items">new items</section><div id="alerts">saved</div><span id="count">3</span>',
      );
    res.setHeader('Content-Type', 'text/html');
    res.end(`<!doctype html>
      <script src="/htmx.js"></script>
      <button id="events" hx-get="/events">events</button>
      <button id="navigate" hx-get="/navigate">go</button>
      <section id="items">old</section>
      <aside id="alerts">old alerts</aside>
      <strong id="count">0</strong>
      <script>
        window.seen = [];
        for (const name of ['z-plain', 'a-target']) {
          document.body.addEventListener(name, (event) => {
            window.seen.push([event.type, event.target.id, event.detail.value]);
          });
        }
      </script>
    `);
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  origin = `http://127.0.0.1:${server.address().port}`;
});
test.afterAll(async () => {
  await new Promise((resolve) => server.close(resolve));
});
test('header Unicode and grouped event targets survive XHR', async ({ page }) => {
  await page.goto(origin);
  await page.locator('#events').click();
  await expect
    .poll(() => page.evaluate(() => seen))
    .toEqual([
      ['z-plain', 'events', 'café 😀'],
      ['a-target', 'alerts', 7],
    ]);
});
test('location selects sibling OOB fragments and replaces final redirected URL', async ({
  page,
}) => {
  await page.goto(origin);
  const length = await page.evaluate(() => history.length);
  await page.locator('#navigate').click();
  await expect(page).toHaveURL(origin + '/destination?q=1');
  await expect(page.locator('#items')).toHaveText('new items');
  await expect(page.locator('div#alerts')).toHaveText('saved');
  await expect(page.locator('strong#count')).toHaveText('3');
  await expect(page.locator('#items #alerts')).toHaveCount(0);
  expect(await page.evaluate(() => history.length)).toBe(length);
});

test('native detail shapes reach browser listeners', async ({ page }) => {
  await page.goto(origin);
  await page.evaluate(() => {
    window.shapes = {};
    for (const name of ['object', 'scalar', 'array', 'nil', 'targeted'])
      document.body.addEventListener(name, (e) => {
        shapes[name] = {
          value: e.detail.value ?? null,
          count: e.detail.count ?? null,
          target: e.target.id,
        };
      });
    htmx.ajax('GET', '/shapes', { source: '#events', target: '#items' });
  });
  await expect.poll(() => page.evaluate(() => Object.keys(shapes).length)).toBe(5);
  expect(await page.evaluate(() => shapes)).toEqual({
    object: { value: null, count: 3, target: 'events' },
    scalar: { value: 7, count: null, target: 'events' },
    array: { value: [1, 2], count: null, target: 'events' },
    nil: { value: null, count: null, target: 'events' },
    targeted: { value: [3, 4], count: null, target: 'alerts' },
  });
});
