using FluentAssertions;
using PraQuando.SharedKernel.Domain;
using PraQuando.SharedKernel.Domain.Errors;
using Xunit;

namespace PraQuando.UnitTests.SharedKernel;

/// <summary>
/// Covers the Map/Bind/Match combinators added to <see cref="Result"/>: successes
/// flow through, failures short-circuit with the original error intact.
/// </summary>
public class ResultCombinatorTests
{
    private static readonly Error TestError = Error.Failure("Test.Failure", "boom");

    [Fact]
    public void Map_projects_the_success_value()
    {
        var result = Result.Success(41).Map(v => v + 1);

        result.IsSuccess.Should().BeTrue();
        result.Value.Should().Be(42);
    }

    [Fact]
    public void Map_propagates_the_failure_error_untouched()
    {
        var result = Result.Failure<int>(TestError).Map(v => v + 1);

        result.IsFailure.Should().BeTrue();
        result.Error.Should().Be(TestError);
    }

    [Fact]
    public void Bind_chains_the_next_operation_on_success()
    {
        var result = Result.Success("category-7")
            .Bind(id => Result.Success(id.Length));

        result.IsSuccess.Should().BeTrue();
        result.Value.Should().Be(10);
    }

    [Fact]
    public void Bind_short_circuits_without_calling_the_continuation()
    {
        var called = false;

        var result = Result.Failure<string>(TestError)
            .Bind(_ => { called = true; return Result.Success(1); });

        called.Should().BeFalse();
        result.Error.Should().Be(TestError);
    }

    [Fact]
    public void Match_selects_the_success_branch()
    {
        var outcome = Result.Success(7).Match(() => "ok", _ => "failed");

        outcome.Should().Be("ok");
    }

    [Fact]
    public void Match_selects_the_failure_branch_with_the_error()
    {
        var outcome = Result.Failure<int>(TestError)
            .Match(() => "ok", error => error.Code);

        outcome.Should().Be("Test.Failure");
    }
}
