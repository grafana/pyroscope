FROM ruby:2.7.4

RUN mkdir -p /usr/src/app
WORKDIR /usr/src/app

ENV RAILS_ENV production
ENV RAILS_SERVE_STATIC_FILES true
ENV RAILS_LOG_TO_STDOUT true

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

EXPOSE 3000

RUN rm -f tmp/pids/server.pid

CMD ["rails", "s", "-b", "0.0.0.0"] 

