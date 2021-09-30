FROM ruby:3

WORKDIR /opt/app

COPY Gemfile ./Gemfile
COPY Gemfile.lock ./Gemfile.lock
# RUN bundle config set --local deployment true
RUN bundle install

COPY scooter/scooter.rb ./scooter/scooter.rb
COPY bike/bike.rb ./bike/bike.rb
COPY car/car.rb ./car/car.rb
COPY utility/utility.rb ./utility/utility.rb

COPY main.rb ./main.rb

CMD [ "ruby", "main.rb" ]
