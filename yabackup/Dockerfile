ARG BUILD_FROM
FROM $BUILD_FROM

# Install requirements for add-on
RUN \
  apk add --no-cache \
    python3 \
    py3-pip

#RUN apt-get -y install python3-pip
RUN pip3 install flask 

# Python 3 HTTP Server serves the current working dir
# So let's set it to our add-on persistent data directory.
WORKDIR /app


# Copy data for add-on
#COPY src/* /
COPY src/ .
RUN chmod a+x ./run.sh

CMD [ "/app/run.sh" ]
