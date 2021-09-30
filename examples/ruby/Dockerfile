FROM ruby:3

WORKDIR /opt/app

COPY Gemfile ./Gemfile
COPY Gemfile.lock ./Gemfile.lock
# RUN bundle config set --local deployment true
RUN bundle install

COPY main.rb ./main.rb

CMD [ "ruby", "main.rb" ]
