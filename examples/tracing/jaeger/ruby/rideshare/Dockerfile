FROM ruby:3.0.2

WORKDIR /rideshare
COPY Gemfile /rideshare/Gemfile
RUN bundle install
COPY app /rideshare/app
COPY config /rideshare/config
COPY public /rideshare/public
