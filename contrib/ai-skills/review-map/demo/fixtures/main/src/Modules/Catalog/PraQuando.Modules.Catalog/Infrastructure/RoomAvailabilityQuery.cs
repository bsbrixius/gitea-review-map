using System.Diagnostics.CodeAnalysis;
using Microsoft.EntityFrameworkCore;
using PraQuando.SharedKernel.Application.CatalogReads;

namespace PraQuando.Modules.Catalog.Infrastructure;

/// <summary>
/// Implementation of <see cref="IRoomAvailabilityQuery"/> — reads from
/// <see cref="Database.CatalogDbContext"/> to look up active rooms within a category.
/// Lives in Catalog.Infrastructure because Catalog owns this data.
/// Instantiated by DI container — suppress CA1812.
/// </summary>
[SuppressMessage("Microsoft.Performance", "CA1812", Justification = "Instantiated by DI container")]
internal sealed class RoomAvailabilityQuery(Database.CatalogDbContext catalogContext) : IRoomAvailabilityQuery
{
    public async Task<List<RoomSlot>> GetAvailableSlotsAsync(Guid categoryId, CancellationToken ct)
    {
        return await catalogContext.Rooms
            .Where(r => r.CategoryId == categoryId && r.IsActive)
            .OrderBy(r => r.Name)
            .Select(r => new RoomSlot(r.Id, r.Name))
            .ToListAsync(ct)
            .ConfigureAwait(false);
    }
}
