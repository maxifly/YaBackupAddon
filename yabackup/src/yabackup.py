from flask import Flask, render_template, request, flash, redirect, url_for
from logging.config import dictConfig

from .root.yad import YaDsk

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
ya_dsk.load_schedule()


@app.route('/')
def index():
    ig_path = get_prefix()
    _LOGGER.info('Root request')
    _LOGGER.debug('Prefix: %s', ig_path)
    if not ya_dsk.ensure_token():
        flash("Token does not exists")

    backup_files = ya_dsk.get_files_info()

    return render_template('index.html', backup_files=backup_files, ig_path=ig_path)


@app.route('/<ig_path>/')
def index_ig(ig_path):
    pass


@app.route('/get_token', methods=('GET', 'POST'))
def get_token():
    ig_path = get_prefix()
    _LOGGER.info('Get token request')

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
    _LOGGER.info('Start upload request')
    return render_template('start_upload.html', ig_path=ig_path)


@app.route('/<ig_path>/start_upload')
def start_upload_ig(ig_path):
    pass


@app.route('/upload1')
def upload():
    ig_path = get_prefix()
    _LOGGER.info('Upload request')

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
