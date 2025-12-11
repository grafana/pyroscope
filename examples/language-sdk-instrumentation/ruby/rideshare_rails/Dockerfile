FROM ruby:3.3.9-slim

# Install runtime and build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    libsqlite3-0 \
    libyaml-0-2 \
    build-essential \
    libsqlite3-dev \
    libyaml-dev \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /usr/src/app
WORKDIR /usr/src/app

ENV RAILS_ENV=production
ENV RAILS_SERVE_STATIC_FILES=true
ENV RAILS_LOG_TO_STDOUT=true

COPY Gemfile /usr/src/app/
COPY Gemfile.lock /usr/src/app/

RUN bundle config --global frozen 1
RUN bundle install --without development test

COPY app /usr/src/app/app
COPY bin /usr/src/app/bin
COPY config /usr/src/app/config
COPY public /usr/src/app/public

COPY Rakefile /usr/src/app/
COPY config.ru /usr/src/app/

EXPOSE 5000

RUN rm -f tmp/pids/server.pid

ENV RIDESHARE_LISTEN_PORT=5000

CMD sh -c "exec rails s -b 0.0.0.0 -p ${RIDESHARE_LISTEN_PORT}"

