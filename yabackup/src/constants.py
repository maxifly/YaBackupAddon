""" All integration constants """
import datetime

FILE_PATH_OPTIONS = "/data/options.json"
FILE_PATH_TOKEN = "/data/token.json"
BACKUP_PATH = "/backup"

OPTION_CLIENT_ID = 'client_id'
OPTION_CLIENT_SECRET = 'client_secret'
OPTION_YD_PATH = 'remote_path'
OPTION_SCHEDULE = 'schedule'

URL_GET_CODE = 'https://oauth.yandex.ru/authorize?response_type=code&client_id='
URL_GET_TOKEN = 'https://oauth.yandex.ru/token'

CONF_TOKEN = 'token'
CONF_REFRESH_TOKEN = 'refresh_token'
CONF_TOKEN_EXPIRES = 'token_expires_date'

REFRESH_TOKEN_DELTA = datetime.timedelta(days=30)