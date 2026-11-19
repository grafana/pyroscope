class WorkController < ApplicationController
  def fast
    render plain: "fast #{burn(0.02)}\n"
  end

  def slow
    render plain: "slow #{burn(0.08)}\n"
  end

  private

  def burn(seconds)
    deadline = Process.clock_gettime(Process::CLOCK_MONOTONIC) + seconds
    iterations = 0
    iterations += 1 while Process.clock_gettime(Process::CLOCK_MONOTONIC) < deadline
    iterations
  end
end
