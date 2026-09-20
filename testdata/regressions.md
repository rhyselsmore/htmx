# Baseline and Regression Cases

These cases cover protocol failures and atomic response updates.

| Permanent regression | Required observation |
| --- | --- |
| Failed detail encoding | Existing events and every header survive; no status/body writes. |
| Numeric accumulation | `9007199254740993` and nested number tokens survive unrelated events. |
| Malformed JSON | An existing malformed object is an error, never a comma-list fallback. |
| Comma event name | One constructor cannot inject several events. |
| Multiple details | Two details fail; neither can be silently ignored. |
| History restoration | Request=true, boosted=false, history=true requests a full page. |
| Invalid location | Unsupported form/header inputs and missing paths return errors atomically. |
| Derived response isolation | Successful, failed, sibling, and chained derivations leave their bases unchanged. |
| Derived event replacement | Payload and target replacement stay within their phase; a plain event clears both. |

The browser baseline serves the pinned asset locally. It asserts Unicode across
XHR, untargeted-before-targeted routing, redirected destination replacement,
OOB wrapper/content semantics, and unchanged history length in three engines.
