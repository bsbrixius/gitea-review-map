namespace PraQuando.SharedKernel.Application.CatalogReads;

/// <summary>
/// Lightweight read model for operating hours. Returned by
/// <see cref="IOperatingHoursQuery"/> — not a Catalog entity.
/// Uses plain Guids to respect the cross-module boundary rule.
/// </summary>
public sealed record OperatingHoursDto(
    Guid CategoryId,
    DayOfWeek DayOfWeek,
    TimeOnly OpensAt,
    TimeOnly ClosesAt,
    bool IsActive)
{
    /// <summary>
    /// True when this entry is active and the given wall-clock time falls inside the
    /// opening window (closing time excluded, matching the slot grid convention).
    /// </summary>
    public bool Covers(TimeOnly time) => IsActive && time >= OpensAt && time < ClosesAt;
}
