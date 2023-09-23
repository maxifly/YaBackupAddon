ARG BUILD_FROM
FROM $BUILD_FROM

# Install requirements for add-on
RUN apk upgrade

RUN apk add --no-cache python3 
RUN apk add --no-cache py3-pip
RUN apk add --no-cache git 
RUN apk add --no-cache supervisor 
RUN apk add --no-cache gcc
RUN apk add --no-cache build-base linux-headers
RUN apk add musl-dev
RUN apk add --no-cache python3-dev
RUN apk add pcre-dev

RUN apk upgrade

RUN pip3 install flask 
RUN pip3 install wheel
RUN pip3 install uwsgi==2.0.19.1
RUN pip3 install yadisk==1.3.0 
RUN pip3 install python-crontab
RUN pip3 install git+https://github.com/coderanger/supervisor-stdout
#RUN pip3 install supervisor-console

# Python 3 HTTP Server serves the current working dir
# So let's set it to our add-on persistent data directory.


WORKDIR /os
COPY os/ .

WORKDIR /app

# Copy data for add-on
COPY app/ .

WORKDIR /app/yabackup

# Copy data for add-on
COPY src/ .

WORKDIR /app

RUN chmod a+x ./run.sh
#RUN crontab /os/cron.txt

CMD ["/usr/bin/supervisord","-c","/os/supervisord.conf"]
#CMD ["/app/run.sh"]