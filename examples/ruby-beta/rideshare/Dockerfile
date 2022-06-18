FROM ruby:3

WORKDIR /opt/app

USER root

RUN gem install pyroscope_beta -v 0.2.3

COPY Gemfile ./Gemfile
COPY Gemfile.lock ./Gemfile.lock
# RUN bundle config set --local deployment true
RUN bundle install

COPY lib ./lib

CMD [ "ruby", "-v", "lib/server.rb" ]
