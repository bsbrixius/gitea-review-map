## Summary

Small, surgical follow-up to the availability work: fluent error combinators on
the shared Result type, two new availability error codes, a cheap room-count
read for callers that only need to tell "no rooms" from "no schedule", and a
past-date guard on the single-date availability entry point. No behavior
change for successful paths; unit tests cover the new combinators.

- Added Map, Bind and Match combinators to Result for fluent error flows
- Introduced availability error codes for past-date windows and inactive categories
- Extended IRoomAvailabilityQuery with CountActiveRoomsAsync and implemented it in Catalog
- Hardened AvailabilityService against past-date queries with richer no-schedule warnings
- Added OperatingHoursDto.Covers for open-window checks
- Added unit tests for the new Result combinators

<!-- make-demo.sh appends the ```review-map block after this comment.
     The block is NOT stored in this template because its `lines:` values are
     computed from the diff the script actually generates (new-side numbers
     shift with every commit, so they must never be hand-maintained here).
     The six bullets above are byte-exact copies of the bullet texts the
     script writes into the block -- edit them only together with make-demo.sh. -->
