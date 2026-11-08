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
    bool IsActive);
