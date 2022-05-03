FROM ruby:3.0.1

WORKDIR /usr/src/app

USER root

#RUN adduser --disabled-password --gecos --quiet pyroscope
#USER pyroscope

#COPY --from=pyroscope/pyroscope:latest /usr/bin/pyroscope /usr/bin/pyroscope

#COPY Gemfile ./Gemfile
#COPY Gemfile.lock ./Gemfile.lock

RUN curl https://sh.rustup.rs -sSf | sh -s -- --default-toolchain stable -y
ENV PATH=$PATH:/root/.cargo/bin

RUN gem install pyroscope_beta

COPY main.rb ./main.rb

CMD ["ruby", "main.rb"]
