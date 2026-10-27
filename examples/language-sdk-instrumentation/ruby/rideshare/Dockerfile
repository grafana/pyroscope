FROM ruby:3.3.13

WORKDIR /opt/app

COPY Gemfile ./Gemfile
COPY Gemfile.lock ./Gemfile.lock
# RUN bundle config set --local deployment true
RUN bundle install

COPY lib ./lib

ENV APP_ENV=production

CMD [ "ruby", "lib/server.rb" ]
