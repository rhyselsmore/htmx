const { test, expect } = require('@playwright/test');
// Query parameters select cases in server/*.go. Each page records event
// snapshots in server/events.js, so assertions observe real client behavior.
const base = () => process.env.HTMX_TEST_BASE_URL;
async function open(page, params = {}, path = '/') {
  await page.goto(base() + path + '?' + new URLSearchParams(params));
  await expect.poll(() => page.evaluate(() => typeof htmx.ajax)).toBe('function');
}
async function click(page) {
  await page.locator('#request').click();
}
async function eventNames(page) {
  return page.evaluate(() => events.map((e) => e.name));
}

test('Go triggers preserve native details and independent targets', async ({ page }) => {
  await open(page, { case: 'events' });
  await click(page);
  await expect.poll(() => eventNames(page)).toContain('b-target');
  const events = await page.evaluate(() => window.events);
  const byName = Object.fromEntries(events.map((event) => [event.name, event]));
  expect(byName['z-plain']).toMatchObject({
    target: 'request',
    value: 'café 😀',
  });
  expect(byName['a-target']).toMatchObject({ target: 'alerts', value: [1, 2] });
  expect(byName['b-target']).toMatchObject({ target: 'count', n: 7 });
  expect(byName.controls.value).toBe('\x7f');
  expect(byName.object.n).toBe(3);
  expect(byName.scalar.value).toBe(7);
  expect(byName.array.value).toEqual([1, 2]);
  expect(byName.nil.value).toBeNull();
  expect(byName.duplicate).toMatchObject({ target: 'request', value: 'new' });
  expect(byName.metadata.value).toEqual({ elt: 'domain', error: 'domain' });
  expect(events.findIndex((e) => e.name === 'z-plain')).toBeLessThan(
    events.findIndex((e) => e.name === 'a-target'),
  );
  await expect(page.locator('#changed')).toHaveText('updated main');
});
// Immediate triggers run even when responseHandling skips the swap. Later
// phases require a swap; opt-in 422 handling exercises both branches.
for (const status of [200, 204, 422]) {
  for (const allow422 of ['0', '1']) {
    test(`status ${status}, 422 configuration ${allow422}: lifecycle follows response handling`, async ({
      page,
    }) => {
      await open(page, { case: 'status', status: String(status), allow422 });
      await click(page);
      await expect.poll(() => page.evaluate(() => requests.length)).toBe(1);
      const swaps = status === 200 || (status === 422 && allow422 === '1');
      if (swaps) {
        await expect.poll(() => eventNames(page)).toContain('after-settle');
        await expect(page.locator('#changed')).toHaveText('updated main');
        const events = await page.evaluate(() => window.events);
        expect(events.map((e) => e.name)).toEqual(['immediate', 'after-swap', 'after-settle']);
        expect(events[0].main).toBe('old main');
        expect(events[1].main).toBe('updated main');
      } else {
        await expect(page.locator('#main')).toHaveText('old main');
        expect(await eventNames(page)).toEqual(['immediate']);
      }
    });
  }
}

test('main none still performs markup OOB work and emits swap-phase events', async ({ page }) => {
  await open(page, { case: 'none' });
  await click(page);
  await expect(page.locator('#alerts')).toHaveText('markup OOB');
  await expect(page.locator('#main')).toHaveText('old main');
  await expect.poll(() => eventNames(page)).toContain('after-settle');
});

for (const ignoreTitle of ['0', '1']) {
  test(`swap delays and title handling ${ignoreTitle}`, async ({ page }) => {
    await open(page, { case: 'modifiers', ignoreTitle, transition: '1' });
    await click(page);
    await expect.poll(() => eventNames(page)).toContain('after-settle');
    const events = await page.evaluate(() => window.events);
    // The server requests 60ms and 40ms delays. Allow timer precision variance
    // while still proving that each phase waits instead of dispatching at once.
    expect(events[1].at - events[0].at).toBeGreaterThanOrEqual(45);
    expect(events[2].at - events[1].at).toBeGreaterThanOrEqual(30);
    await expect(page).toHaveTitle(ignoreTitle === '1' ? 'original title' : 'new title');
  });
}

for (const redirect of ['0', '1']) {
  test(`destination replacement resolves final URL, redirect=${redirect}`, async ({ page }) => {
    await page.goto(base() + '/previous');
    await open(page, {
      case: 'location',
      history: 'destination',
      redirect,
      anchor: 'anchor',
    });
    const length = await page.evaluate(() => history.length);
    await click(page);
    await expect(page.locator('#items')).toContainText('new items');
    await expect.poll(() => page.evaluate(() => location.pathname)).toBe('/destination');
    expect(await page.evaluate(() => location.hash)).toBe('#anchor');
    expect(await page.evaluate(() => new URL(location.href).searchParams.get('redirect'))).toBe(
      redirect,
    );
    expect(await page.evaluate(() => history.length)).toBe(length);
    await page.goBack();
    await expect(page).toHaveURL(base() + '/previous');
    await expect(page.locator('#items')).toHaveText('old items');
  });
}

for (const history of ['default', 'push', 'replace', 'destination', 'suppress']) {
  for (const boosted of ['0', '1']) {
    test(`location history ${history}, boosted=${boosted}`, async ({ page }) => {
      await open(page, {
        case: 'location',
        history,
        boosted,
        inheritPush: '/inherited-push',
        inheritReplace: '/inherited-replace',
      });
      const before = page.url();
      const length = await page.evaluate(() => window.history.length);
      await click(page);
      await expect(page.locator('#items')).toContainText('new items');
      // Suppression falls through to a push for boosted requests in 2.0.10.
      const expectedPaths = {
        default: '/destination',
        push: '/chosen-push',
        replace: '/chosen-replace',
        destination: '/destination',
        suppress: boosted === '1' ? '/destination' : new URL(before).pathname,
      };
      const path = expectedPaths[history];
      await expect.poll(() => page.evaluate(() => location.pathname)).toBe(path);
      const pushes =
        history === 'default' || history === 'push' || (history === 'suppress' && boosted === '1');
      expect(await page.evaluate(() => window.history.length)).toBe(length + (pushes ? 1 : 0));
      if (history === 'suppress' && boosted === '0') expect(page.url()).toBe(before);
      await expect(page.locator('#outside')).toHaveText('untouched');
    });
  }
}

for (const override of ['suppress', 'push', 'replace']) {
  test(`follow-up history header overrides location: ${override}`, async ({ page }) => {
    await open(page, {
      case: 'location',
      history: 'destination',
      override,
      boosted: '1',
    });
    const before = page.url();
    const length = await page.evaluate(() => history.length);
    await click(page);
    await expect(page.locator('#items')).toContainText('new items');
    if (override === 'suppress') expect(page.url()).toBe(before);
    else
      await expect.poll(() => page.evaluate(() => location.pathname)).toBe('/override-' + override);
    expect(await page.evaluate(() => history.length)).toBe(length + (override === 'push' ? 1 : 0));
  });
}

for (const strategy of [
  'outerHTML',
  'innerHTML',
  'beforebegin',
  'afterbegin',
  'beforeend',
  'afterend',
  'delete',
  'none',
]) {
  test(`OOB ${strategy} changes the correct wrapper or contents`, async ({ page }) => {
    await open(page, { case: 'location', history: 'suppress', strategy });
    await page.evaluate(() => (window.originalAlert = document.querySelector('#alerts')));
    await click(page);
    await expect(page.locator('#count')).toHaveText('3');
    await expect(page.locator('#items')).toContainText('new items');
    await expect(page.locator('#items #alerts')).toHaveCount(0);
    await expect(page.locator('#outside')).toHaveText('untouched');
    if (strategy === 'delete') {
      await expect(page.locator('#alerts')).toHaveCount(0);
      await expect(page.locator('#new-alert')).toHaveCount(0);
    } else if (strategy === 'none') {
      await expect(page.locator('#old-alert')).toHaveText('old alert');
      await expect(page.locator('#new-alert')).toHaveCount(0);
    } else {
      await expect(page.locator('#new-alert')).toHaveText('saved');
      expect(await page.evaluate(() => document.querySelector('#alerts') === originalAlert)).toBe(
        strategy !== 'outerHTML',
      );
      // Assert wrapper identity above and exact insertion position here.
      const expectedSelectors = {
        outerHTML: 'div#alerts',
        innerHTML: 'aside#alerts > #new-alert',
        beforebegin: '#new-alert + #alerts',
        afterend: '#alerts + #new-alert',
        afterbegin: '#alerts > #new-alert + #old-alert',
        beforeend: '#alerts > #old-alert + #new-alert',
      };
      await expect(page.locator(expectedSelectors[strategy])).toHaveCount(1);
    }
    expect(await page.evaluate(() => document.querySelector('#count').tagName)).toBe('STRONG');
    expect(
      await page.evaluate(() =>
        lifecycle.some((e) => e.name === 'htmx:oobAfterSwap' && e.swapTarget === 'count'),
      ),
    ).toBe(true);
  });
}

for (const missing of ['source', 'target']) {
  test(`OOB missing ${missing}`, async ({ page }) => {
    await open(page, { case: 'location', missing });
    await click(page);
    await expect(page.locator('#count')).toHaveText('3');
    expect(await page.evaluate(() => errors)).toEqual(
      missing === 'target' ? ['oobErrorNoTarget'] : [],
    );
    await expect(page.locator('#ghost')).toHaveCount(0);
  });
}

for (const order of ['parent-first', 'child-first']) {
  test(`OOB preserves source order ${order}`, async ({ page }) => {
    await open(page, { case: 'location', order });
    await click(page);
    await expect(page.locator('#items')).toContainText('new items');
    // Selecting the parent consumes the nested child source too. Selecting the
    // child first moves it out, leaving an empty parent for the second swap.
    if (order === 'parent-first') {
      await expect(page.locator('#parent > #child')).toHaveText('new child');
      await expect(page.locator('body > section#child')).toHaveText('old child');
    } else {
      await expect(page.locator('#parent')).toBeEmpty();
      await expect(page.locator('body > div#child')).toHaveText('new child');
    }
  });
}

test('main none permits location OOB work', async ({ page }) => {
  await open(page, { case: 'location', mainNone: '1' });
  await click(page);
  await expect(page.locator('#count')).toHaveText('3');
  await expect(page.locator('#items')).toHaveText('old items');
});

test('empty location OOB option preserves inherited selection', async ({ page }) => {
  await open(page, {
    case: 'location',
    omitOOB: '1',
    inheritOOB: '#alerts:outerHTML',
  });
  await click(page);
  await expect(page.locator('div#alerts')).toHaveText('saved');
  await expect(page.locator('#count')).toHaveText('0');
});

test('location form parameters repeat and custom header names work', async ({ page }) => {
  await open(page, { case: 'location', form: '1' });
  await click(page);
  await expect(page.locator('#params')).toBeVisible();
  const parameters = JSON.parse(await page.locator('#params').textContent());
  expect(parameters.query.tag).toEqual(['go', 'café']);
  expect(parameters.query.id).toEqual(['9007199254740993']);
  expect(parameters.custom).toBe('yes');
  expect(parameters.property).toBe('safe');
});

for (const restoreHx of ['true', 'false']) {
  test(`history cache miss requests full HTML, historyRestoreAsHxRequest=${restoreHx}`, async ({
    page,
  }) => {
    await open(page, { restoreHx }, '/history/start');
    await click(page);
    await expect(page.locator('#next')).toHaveText('next fragment');
    const request = page.waitForRequest(
      (r) => r.headers()['hx-history-restore-request'] === 'true',
    );
    await page.goBack();
    const restored = await request;
    expect(restored.headers()['hx-request'] ?? 'false').toBe(restoreHx);
    await expect(page.locator('#restore-result')).toHaveAttribute('data-restore', 'true');
    await expect(page.locator('#restore-result')).toHaveAttribute('data-fragment', 'false');
  });
}

test('boosted navigation gets a full-page response', async ({ page }) => {
  await open(page, { boosted: '1' }, '/history/start');
  const response = page.waitForResponse((r) => new URL(r.url()).pathname === '/history/next');
  await click(page);
  expect((await response).headers()['x-wants-fragment']).toBe('false');
  await expect(page.locator('#next')).toHaveText('next full page');
});

test('Redirect performs full navigation; Refresh reloads the current page', async ({ page }) => {
  await open(page, { case: 'redirect' });
  await click(page);
  await expect(page).toHaveURL(base() + '/landed');
  await open(page, { case: 'refresh' });
  await page.evaluate(() => (window.marker = 'before'));
  const navigation = page.waitForEvent('framenavigated', (f) => f === page.mainFrame());
  await click(page);
  await navigation;
  await expect.poll(() => page.evaluate(() => window.marker)).toBeUndefined();
  await expect(page.locator('#main')).toHaveText('old main');
});

test('intermediate HTTP redirect headers are not handled', async ({ page }) => {
  await open(page, { case: 'http-redirect' });
  await click(page);
  await expect(page.locator('#final')).toHaveText('final');
  expect(await eventNames(page)).toEqual(['final-redirect']);
});

for (const skip286 of ['0', '1']) {
  test(`286 polling cancellation, swap disabled=${skip286}`, async ({ page }) => {
    // Advance browser timers explicitly, then wait for real HTTP completion.
    // Counting server requests distinguishes stopped polling from skipped swaps.
    await page.clock.install({ time: new Date('2024-01-01T00:00:00Z') });
    await page.clock.pauseAt(new Date('2024-01-01T01:00:00Z'));
    const token = String(Date.now()) + '-' + Math.random();
    await open(page, { case: 'poll', token, skip286 });
    for (let n = 1; n <= 2; n++) {
      await page.clock.runFor(60);
      await expect.poll(() => page.evaluate(() => requests.length)).toBe(n);
    }
    await page.clock.runFor(500);
    const count = async () => {
      const response = await page.request.get(
        base() + '/poll-count?token=' + encodeURIComponent(token),
      );
      return Number(await response.text());
    };
    if (skip286 === '0') expect(await count()).toBe(2);
    else await expect.poll(count).toBeGreaterThan(2);
  });
}

for (const phase of ['immediate', 'swap', 'settle']) {
  test(`mixed trigger routing in ${phase} phase`, async ({ page }) => {
    await open(page, { case: 'phase-routing', phase });
    await click(page);
    await expect.poll(() => eventNames(page)).toHaveLength(2);
    expect(
      await page.evaluate(() => events.map(({ name, target, value }) => ({ name, target, value }))),
    ).toEqual([
      { name: 'z-plain', target: 'request', value: 'ordinary' },
      { name: 'a-target', target: 'alerts', value: 7 },
    ]);
  });
}

test('scroll selector colons and Show position resolve in the browser', async ({ page }) => {
  await open(page, { case: 'scroll' });
  await click(page);
  await expect(page.locator('#changed')).toHaveText('updated main');
  await expect
    .poll(() => page.locator('#scroll-box').evaluate((el) => el.scrollTop))
    .toBeGreaterThan(900);
  await expect
    .poll(() =>
      page
        .locator('#show-target')
        .evaluate((el) => Math.abs(el.getBoundingClientRect().bottom - innerHeight)),
    )
    .toBeLessThan(3);
});

// Reuse one server-side base across XHRs. Assertions cover routing and rendered
// content, so a correctly formatted header with leaked state still fails.
test('derived responses keep per-request details and targets out of their shared base', async ({
  page,
}) => {
  await open(page, { case: 'derived' });
  const cases = [
    { value: 'first café 😀', target: '#alerts', eventTarget: 'alerts' },
    { value: 'second', target: '#count', eventTarget: 'count' },
    { value: 'plain', mode: 'plain', eventTarget: 'request', eventValue: null },
    { value: 'base', mode: 'base', eventTarget: 'alerts' },
  ];
  for (const entry of cases) {
    const query = new URLSearchParams({ case: 'derived', value: entry.value });
    if (entry.target) query.set('target', entry.target);
    if (entry.mode) query.set('mode', entry.mode);
    await page.evaluate(async (query) => {
      window.events = [];
      await htmx.ajax('GET', '/response?' + query, { source: '#request', target: '#main' });
    }, query.toString());
    await expect(page.locator('#changed')).toHaveText(entry.value);
    await expect.poll(() => eventNames(page)).toContain('after-swap');
    const events = await page.evaluate(() => window.events);
    expect(events).toHaveLength(3);
    expect(events.find((event) => event.name === 'a-target')).toMatchObject({
      target: entry.eventTarget,
      value: entry.eventValue === null ? null : entry.value,
    });
    expect(events.find((event) => event.name === 'z-plain')).toMatchObject({
      target: 'request',
      value: 'ordinary',
    });
    expect(events.find((event) => event.name === 'after-swap')).toMatchObject({
      target: 'request',
      value: entry.value,
      main: entry.value,
    });
  }
});
