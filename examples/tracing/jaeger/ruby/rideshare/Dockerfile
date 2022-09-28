FROM ruby:3.0.2

RUN git clone https://github.com/pyroscope-io/otel-profiling-ruby.git \
    && cd otel-profiling-ruby \
    && git checkout 62caca3 \
    && bundle install \
    && rake install
WORKDIR /rideshare
COPY Gemfile /rideshare/Gemfile
RUN bundle install
COPY bin /rideshare/bin
COPY app /rideshare/app
COPY db /rideshare/db
COPY config /rideshare/config
COPY public /rideshare/public
COPY config.ru .ruby-version Rakefile /rideshare/

EXPOSE 5000

CMD rails server -b 0.0.0.0 -p 5000
