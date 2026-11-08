using System.Diagnostics.CodeAnalysis;

namespace PraQuando.SharedKernel.Domain.Errors;

/// <summary>
/// Central registry for error-code strings passed to
/// <see cref="Error.Failure(string, string)"/> /
/// <see cref="Error.NotFound(string, string)"/> /
/// <see cref="Error.Conflict(string, string)"/>.
///
/// The <c>Code</c> is a stable wire identifier returned in ProblemDetails
/// (<c>error.code</c>); tests and API clients key off it. Naming convention:
/// <c>{Module}.{Error}</c> in PascalCase, mirroring the bounded-context structure.
/// Payments codes live in <c>PaymentsErrorCodes</c> (Payments.Application) — the
/// Payments module owns its own registry.
///
/// See <c>.agents/skills/constant-registry/SKILL.md</c>.
/// </summary>
[SuppressMessage("Design", "CA1034:Nested types should not be visible", Justification = "Nested classes group error codes by module for discoverability (ErrorCodes.Module.Error).")]
public static class ErrorCodes
{
    // ── General / SharedKernel ──────────────────────────────────────────
    public static class General
    {
        public const string NullValue = "General.Null";
    }

    // ── Booking ─────────────────────────────────────────────────────────
    public static class Booking
    {
        public const string NotFound = "Booking.NotFound";
        public const string SlotConflict = "Booking.SlotConflict";
        public const string NoRoomAvailable = "Booking.NoRoomAvailable";
        public const string UpdateConflict = "Booking.UpdateConflict";
        public const string InvalidTransition = "Booking.InvalidTransition";
        public const string NotProvisional = "Booking.NotProvisional";
        public const string NotCancellable = "Booking.NotCancellable";
        public const string ConfirmConflict = "Booking.ConfirmConflict";
        public const string CancelConflict = "Booking.CancelConflict";
        public const string SlotNotOnGrid = "Booking.SlotNotOnGrid";
        public const string InvalidDateRange = "Booking.InvalidDateRange";
        public const string InvalidTimeZone = "Booking.InvalidTimeZone";
        public const string AvailabilityWindowInPast = "Booking.AvailabilityWindowInPast";
    }

    // ── Catalog ─────────────────────────────────────────────────────────
    public static class Catalog
    {
        public const string CategoryNotFound = "Catalog.CategoryNotFound";
        public const string RoomNotFound = "Catalog.RoomNotFound";
        public const string CategoryNotActive = "Catalog.CategoryNotActive";
    }
}
