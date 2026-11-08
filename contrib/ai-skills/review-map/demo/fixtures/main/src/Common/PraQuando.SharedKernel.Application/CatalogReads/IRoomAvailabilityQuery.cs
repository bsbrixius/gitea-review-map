namespace PraQuando.SharedKernel.Application.CatalogReads;

/// <summary>
/// Cross-module read-only query to look up available rooms within a category.
/// This is a neutral contract in SharedKernel — both the owning module
/// (Catalog.Infrastructure) and consuming modules (Booking.Application)
/// depend on this interface, never on each other.
/// </summary>
public interface IRoomAvailabilityQuery
{
    Task<List<RoomSlot>> GetAvailableSlotsAsync(Guid categoryId, CancellationToken ct);
}
