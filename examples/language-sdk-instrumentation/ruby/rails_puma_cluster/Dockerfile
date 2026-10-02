FROM ruby:3.3.9-slim

RUN mkdir -p /usr/src/app
WORKDIR /usr/src/app

ENV RAILS_ENV=production
ENV RAILS_LOG_TO_STDOUT=true

COPY Gemfile /usr/src/app/
COPY Gemfile.lock /usr/src/app/

# Build deps are installed, used and purged in one layer so the compiler and
# linux-libc-dev never reach the runtime image.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
    && bundle config --global frozen 1 \
    && bundle install --without development test \
    && apt-get purge -y --auto-remove \
        build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY app /usr/src/app/app
COPY config /usr/src/app/config
COPY config.ru /usr/src/app/

EXPOSE 5000

CMD ["bundle", "exec", "puma", "-C", "config/puma.rb"]
