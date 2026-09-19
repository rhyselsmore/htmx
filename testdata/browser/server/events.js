// A boosted full-page swap may execute this script again. Keep declarations
// local while replacing the observation arrays for the newly rendered page.
(() => {
  // Observe the real DOM events without retaining live elements. Tests can then
  // compare snapshots even after a swap removes or replaces the original target.
  window.events = [];
  window.lifecycle = [];
  window.errors = [];
  window.requests = [];

  const responseEvents = [
    'immediate',
    'after-swap',
    'after-settle',
    'z-plain',
    'a-target',
    'b-target',
    'object',
    'scalar',
    'array',
    'nil',
    'duplicate',
    'metadata',
    'controls',
    'done',
    'ignored-redirect',
    'final-redirect',
  ];

  for (const name of responseEvents) {
    document.body.addEventListener(name, (event) => {
      window.events.push({
        name,
        target: event.target.id,
        value: event.detail.value ?? null,
        n: event.detail.n ?? null,
        at: performance.now(),
        // This captures whether the main swap has happened at this trigger phase.
        main: document.querySelector('#main')?.textContent,
      });
    });
  }

  const lifecycleEvents = [
    'htmx:beforeRequest',
    'htmx:afterRequest',
    'htmx:afterSwap',
    'htmx:afterSettle',
    'htmx:oobAfterSwap',
  ];

  for (const name of lifecycleEvents) {
    document.body.addEventListener(name, (event) => {
      window.lifecycle.push({
        name,
        at: performance.now(),
        target: event.target.id,
        swapTarget: event.detail?.target?.id,
      });
    });
  }

  document.body.addEventListener('htmx:oobErrorNoTarget', () => {
    window.errors.push('oobErrorNoTarget');
  });

  // A completed XHR is observable even when responseHandling disables its swap.
  document.body.addEventListener('htmx:afterRequest', (event) => {
    window.requests.push(event.detail.xhr.status);
  });
})();
