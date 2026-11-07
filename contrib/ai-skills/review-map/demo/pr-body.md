## Summary

This PR adds the Advanced Query feature end to end: store slice and selectors,
API client, infinite row datasource integration, hook and component.

- Added Advanced Query slice and selectors to manage state and criteria
- Updated infinite row datasource to support fetching data from the Advanced Query endpoint
- Added queryClient API wrapper with batching, retry and typed error handling
- Added useAdvancedQuery hook with polling and cache invalidation
- Reworked AppState slice to track query lifecycle bookkeeping
- Added QueryBuilder component with criteria editing and validation

<!-- make-demo.sh appends the ```review-map block after this comment.
     The block is NOT stored in this template because its `lines:` values are
     computed from the diff the script actually generates (new-side numbers
     shift with every commit, so they must never be hand-maintained here).
     The six bullets above are byte-exact copies of the bullet texts the
     script writes into the block -- edit them only together with make-demo.sh. -->
