import datetime
import time

from flask import Flask, render_template, request, flash, redirect, url_for
from logging.config import dictConfig
import logging

from .yad import YaDsk

dictConfig(
    {
        "version": 1,
        "formatters": {
            "default": {
                "format": "[%(asctime)s] %(levelname)s in %(module)s: %(message)s",
            }
        },
        "handlers": {
            "console": {
                "class": "logging.StreamHandler",
                "stream": "ext://sys.stdout",
                "formatter": "default",
                "level": "INFO"
            },
            "file": {
                "class": "logging.FileHandler",
                "filename": "flask.log",
                "formatter": "default",
                "level": "DEBUG"
            }
        },
        "root": {"level": "DEBUG", "handlers": ["console", "file"]},

    }
)

app = Flask(__name__)
app.config['SECRET_KEY'] = 'your secret key'
_LOGGER = app.logger
ya_dsk = YaDsk(_LOGGER)


@app.route('/')
def index():
    _LOGGER.info("Logger name %s", _LOGGER.name)
    _LOGGER.info("Logger handlers %s", _LOGGER.handlers)
    _LOGGER.info("App name %s", app.name)
    _LOGGER.info("App logger name %s", app.logger.name)
    _LOGGER.info("App logger handlers %s", app.logger.handlers)
    _LOGGER.debug("debug log info")
    _LOGGER.info("Info log information")
    _LOGGER.warning("Warning log info")
    _LOGGER.error("Error log info")
    _LOGGER.critical("Critical log info")

    ig_path = get_prefix()
    _LOGGER.info('root hhh %s', ig_path)
    if not ya_dsk.ensure_token():
        flash("Token does not exists")

    ya_dsk.load_schedule()

    backup_files = ya_dsk.get_files_info()

    return render_template('index.html', backup_files=backup_files, ig_path=ig_path)


@app.route('/<ig_path>/')
def index_ig(ig_path):
    pass


@app.route('/get_token', methods=('GET', 'POST'))
def get_token():
    ig_path = get_prefix()
    _LOGGER.info('get token hhh %s', ig_path)

    if request.method == 'POST':
        check_code = request.form['check_code']

        if not check_code:
            flash('Check code is required!')
        else:
            token_created = False
            try:
                ya_dsk.create_token(check_code)
                token_created = True
            except Exception as e:
                flash("Get token exception " + str(e))

            if token_created:
                return redirect(url_for('index_ig', ig_path=ig_path))
    return render_template('get_token.html', url_check_code=ya_dsk.getCheckCodeUrl(), ig_path=ig_path)


@app.route('/<ig_path>/get_token', methods=('GET', 'POST'))
def get_token_ig(ig_path):
    pass


@app.route('/start_upload')
def start_upload():
    ig_path = get_prefix()
    _LOGGER.info('start upload hhh %s', ig_path)
    return render_template('start_upload.html', ig_path=ig_path)


@app.route('/<ig_path>/start_upload')
def start_upload_ig(ig_path):
    pass


@app.route('/upload1')
def upload():
    ig_path = get_prefix()
    _LOGGER.info('upload hhh %s', ig_path)

    time.sleep(30)
    ya_dsk.upload_files()
    return redirect(url_for('index_ig', ig_path=ig_path))


@app.route('/<ig_path>/upload1')
def upload_ig(ig_path):
    pass


def get_prefix():
    result = request.headers.get('X-Ingress-Path')
    if result:
        return result
    return ''
