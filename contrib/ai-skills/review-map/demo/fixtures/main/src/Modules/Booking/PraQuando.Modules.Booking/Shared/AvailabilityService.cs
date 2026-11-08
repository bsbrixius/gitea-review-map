using System.Diagnostics.CodeAnalysis;
using Microsoft.Extensions.Logging;
using PraQuando.Modules.Booking.Abstractions;
using PraQuando.Modules.Booking.Domain.Entities;
using PraQuando.Modules.Booking.Domain.Schedules;
using PraQuando.Modules.Booking.Domain.Scheduling;
using PraQuando.SharedKernel.Application.CatalogReads;
using PraQuando.SharedKernel.Domain;
using PraQuando.SharedKernel.Domain.Errors;

namespace PraQuando.Modules.Booking.Shared;

using BookingStatus = PraQuando.Modules.Booking.Domain.Entities.BookingStatus;
using ScheduleEntity = PraQuando.Modules.Booking.Domain.Schedules.Schedule;

/// <summary>
/// Computes available time slots for a (category, date) pair on the engine
/// pipeline: discrete grid slots via <see cref="SlotGenerator"/> from the
/// category's <see cref="ScheduleEntity"/> assignment + <see cref="CategorySlotPolicy"/>,
/// surfacing taken starts alongside free ones (spec section 6.4). A category
/// without an assignment (or policy) yields an empty result with a warning -
/// the dev-bootstrap backfill assigns a schedule to every seeded category.
/// </summary>
/// <remarks>
/// Follows the same composition pattern as <see cref="BookingAllocationService"/>:
/// constructor injection, <see cref="Result{T}"/> return, Scoped lifetime.
/// </remarks>
[SuppressMessage("Microsoft.Performance", "CA1848", Justification = "Two warn-only logs at a read boundary; LoggerMessage source generator not worth the ceremony - matches the Payments service posture (PaymentService) and the Host csproj.")]
public sealed class AvailabilityService
{
    private const int MaxRangeDays = 62;
    private const string FallbackTimeZoneId = "UTC";

    private readonly IRoomAvailabilityQuery _roomQuery;
    private readonly IBookingRepository _bookingRepository;
    private readonly IOperatingHoursQuery _operatingHoursQuery;
    private readonly TimeProvider _timeProvider;
    private readonly IScheduleRepository _schedules;
    private readonly ICategoryPolicyRepository _policies;
    private readonly ILogger<AvailabilityService> _logger;

    public AvailabilityService(
        IRoomAvailabilityQuery roomQuery,
        IBookingRepository bookingRepository,
        IOperatingHoursQuery operatingHoursQuery,
        TimeProvider timeProvider,
        IScheduleRepository schedules,
        ICategoryPolicyRepository policies,
        ILogger<AvailabilityService> logger)
    {
        _roomQuery = roomQuery;
        _bookingRepository = bookingRepository;
        _operatingHoursQuery = operatingHoursQuery;
        _timeProvider = timeProvider;
        _schedules = schedules;
        _policies = policies;
        _logger = logger;
    }

    /// <summary>
    /// Computes priced grid slots for all rooms in a category on the given date
    /// (single-date form): delegates to the engine core for that one date.
    /// A category without a schedule assignment (or slot policy) yields an
    /// empty list with a warning. Success -> priced list, failure -> propagate.
    /// </summary>
    public async Task<Result<List<AvailabilitySlotDto>>> GetAvailabilityWithPriceAsync(
        Guid categoryId,
        DateTime dateUtc,
        CancellationToken ct)
    {
        var schedule = await _schedules
            .GetByCategoryAsync(categoryId, ct)
            .ConfigureAwait(false);

        CategorySlotPolicy? policy = null;
        if (schedule is not null)
            policy = await _policies
                .GetByCategoryAsync(categoryId, ct)
                .ConfigureAwait(false);

        if (schedule is null || policy is null)
        {
            _logger.LogWarning(
                "No schedule/policy assigned for category {CategoryId} - availability empty",
                categoryId);
            return Result.Success(new List<AvailabilitySlotDto>());
        }

        var date = DateOnly.FromDateTime(dateUtc.Date);
        var range = await BuildEngineRangeAsync(
            categoryId, date, date, timeZoneId: null, schedule, policy, ct)
            .ConfigureAwait(false);

        return range.IsSuccess
            ? Result.Success(range.Value.Slots.ToList())
            : Result.Failure<List<AvailabilitySlotDto>>(range.Error);
    }

    /// <summary>
    /// Range availability over the engine (spec section 6.4). Dates are schedule-local;
    /// instants are UTC. A category without a schedule assignment (or policy) yields
    /// an empty envelope with a warning.
    /// </summary>
    public async Task<Result<AvailabilityRangeDto>> GetAvailabilityRangeAsync(
        Guid categoryId,
        DateOnly from,
        DateOnly to,
        string? timeZoneId,
        CancellationToken ct)
    {
        if (from > to ||
            to.ToDateTime(TimeOnly.MinValue) - from.ToDateTime(TimeOnly.MinValue) > TimeSpan.FromDays(MaxRangeDays))
            return Result.Failure<AvailabilityRangeDto>(new Error(
                ErrorCodes.Booking.InvalidDateRange,
                $"'from' must not be after 'to' and the range must not exceed {MaxRangeDays} days.",
                ErrorType.Validation));

        var schedule = await _schedules.GetByCategoryAsync(categoryId, ct).ConfigureAwait(false);
        var policy = await _policies.GetByCategoryAsync(categoryId, ct).ConfigureAwait(false);

        var tzId = timeZoneId ?? schedule?.TimeZoneId ?? FallbackTimeZoneId;
        if (TimeZoneInfo.TryFindSystemTimeZoneById(tzId, out _) is false)
            return Result.Failure<AvailabilityRangeDto>(new Error(
                ErrorCodes.Booking.InvalidTimeZone,
                $"Unknown timezone id '{tzId}'.",
                ErrorType.Validation));

        if (schedule is null || policy is null)
        {
            _logger.LogWarning(
                "No schedule/policy assigned for category {CategoryId} - availability empty",
                categoryId);
            return Result.Success(new AvailabilityRangeDto([], []));
        }

        return await BuildEngineRangeAsync(categoryId, from, to, timeZoneId, schedule, policy, ct)
            .ConfigureAwait(false);
    }

    /// <summary>
    /// v2 engine pipeline for categories with a schedule + policy: translates each room's
    /// schedule (with buffer-expanded busy intervals) into <see cref="ResourceAvailability"/>,
    /// generates the grid, prices every slot at rate x duration, and reports taken starts.
    /// The timezone is re-validated here because the single-date delegation from
    /// <see cref="GetAvailabilityWithPriceAsync"/> never passes an explicit id.
    /// </summary>
    private async Task<Result<AvailabilityRangeDto>> BuildEngineRangeAsync(
        Guid categoryId,
        DateOnly from,
        DateOnly to,
        string? timeZoneId,
        ScheduleEntity schedule,
        CategorySlotPolicy policy,
        CancellationToken ct)
    {
        var tzId = timeZoneId ?? schedule.TimeZoneId;
        if (TimeZoneInfo.TryFindSystemTimeZoneById(tzId, out _) is false)
            return Result.Failure<AvailabilityRangeDto>(new Error(
                ErrorCodes.Booking.InvalidTimeZone,
                $"Unknown timezone id '{tzId}'.",
                ErrorType.Validation));

        var rooms = await _roomQuery.GetAvailableSlotsAsync(categoryId, ct).ConfigureAwait(false);
        if (rooms.Count == 0)
            return Result.Success(new AvailabilityRangeDto([], []));

        var nowOffset = new DateTimeOffset(_timeProvider.GetUtcNow().UtcDateTime, TimeSpan.Zero);
        var rate = await _operatingHoursQuery
            .GetPricePerHourAsync(categoryId, ct)
            .ConfigureAwait(false) ?? 0m;
        var config = new SlotGridConfig(
            policy.DurationMinutes, policy.IntervalMinutes, policy.MinNoticeMinutes, policy.MaxFutureDays);

        var resourceAvailability = new List<ResourceAvailability>();
        foreach (var room in rooms)
        {
            // One booking window spanning the whole range (+/- a day for cross-midnight
            // timezone offsets); the engine clips per-day windows itself.
            // SpecifyKind(Utc): DateOnly.ToDateTime yields Kind=Unspecified, which
            // Npgsql's timestamptz rejects (range queries 500'd before this).
            var dayStart = DateTime.SpecifyKind(
                from.ToDateTime(TimeOnly.MinValue).AddDays(-1), DateTimeKind.Utc);
            var dayEnd = DateTime.SpecifyKind(
                to.ToDateTime(TimeOnly.MinValue).AddDays(2), DateTimeKind.Utc);
            var bookings = await _bookingRepository.GetBookingsForRoomOnDateAsync(
                room.RoomId, dayStart, dayEnd, nowOffset.UtcDateTime, ct).ConfigureAwait(false);

            var buffer = await _bookingRepository
                .GetActiveBufferRuleAsync(room.RoomId, ct)
                .ConfigureAwait(false);

            var busy = bookings
                .Where(b => b.Status != BookingStatus.Cancelled)
                .Select(b => new DateInterval(
                    new DateTimeOffset(b.StartsAtUtc.AddMinutes(-(buffer?.BufferMinutesBefore ?? 0)), TimeSpan.Zero),
                    new DateTimeOffset(b.EndsAtUtc.AddMinutes(buffer?.BufferMinutesAfter ?? 0), TimeSpan.Zero)))
                .ToList();

            resourceAvailability.Add(ScheduleTranslator.ToResourceAvailability(schedule, room.RoomId, busy));
        }

        var (free, taken) = SlotGenerator.GenerateWithTaken(config, resourceAvailability, from, to, nowOffset);
        var durationHours = (decimal)policy.DurationMinutes / 60m;

        // Distinct-by-start: a start is offered when ANY room in the category is free.
        var slots = free.Select(s => s.Start).Distinct()
            .Select(start => new AvailabilitySlotDto(
                start.UtcDateTime,
                start.AddMinutes(policy.DurationMinutes).UtcDateTime,
                Math.Round(rate * durationHours, 2)))
            .OrderBy(s => s.StartsAtUtc)
            .ToList();

        var takenDtos = taken
            .Select(t => new TakenSlotDto(t.Start.UtcDateTime, t.End.UtcDateTime))
            .ToList();

        return Result.Success(new AvailabilityRangeDto(slots, takenDtos));
    }
}
