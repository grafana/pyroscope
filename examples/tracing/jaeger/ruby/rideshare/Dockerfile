FROM ruby:3.0.2

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
