class PyroscopeRouteTaggingMiddleware
  FORMAT_SUFFIX = /\(\.:format\)\z/.freeze

  def initialize(app)
    @app = app
  end

  def call(env)
    pattern = route_pattern_for(env)
    return @app.call(env) unless pattern

    Pyroscope.tag_wrapper(
      "route" => pattern,
      "method" => env["REQUEST_METHOD"],
    ) do
      @app.call(env)
    end
  end

  private

  def route_pattern_for(env)
    req = ActionDispatch::Request.new(env)
    Rails.application.routes.router.recognize(req) do |route, _params|
      return normalize(route.path.spec.to_s)
    end
    nil
  rescue StandardError
    nil
  end

  def normalize(pattern)
    pattern.sub(FORMAT_SUFFIX, "")
  end
end
