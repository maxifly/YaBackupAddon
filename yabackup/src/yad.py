import json
import logging
import datetime
import time
from typing import Generator


import yadisk as yadisk
from crontab import CronTab

from yadisk import YaDisk
from yadisk.objects import TokenObject, ResourceObject

from .classes import BackupFile
from .constants import FILE_PATH_OPTIONS, OPTION_CLIENT_ID, OPTION_CLIENT_SECRET, URL_GET_CODE, \
    CONF_TOKEN, CONF_REFRESH_TOKEN, CONF_TOKEN_EXPIRES, FILE_PATH_TOKEN, BACKUP_PATH, OPTION_YD_PATH, \
    REFRESH_TOKEN_DELTA, OPTION_SCHEDULE, OPTION_REMOTE_MAX_QUANTITY
from .bkp_observer import BackupObserver, Backup, byte_to_mb

TYPE_FILE = 'file'


def read_options():
    """ Read options """
    with open(FILE_PATH_OPTIONS) as f:
        d = json.load(f)
        return d


def dump_token(token_info):
    """ Dump token information into file """
    with open(FILE_PATH_TOKEN, 'w', encoding='utf-8') as f:
        f.write(json.dumps(token_info, ensure_ascii=False))


def read_token():
    """ Read token from file """
    try:
        with open(FILE_PATH_TOKEN, 'r', encoding='utf-8') as f:
            token_info = json.load(f)
            return token_info
    except FileNotFoundError as e:
        return None


def get_remote_file_name_from_local(local_file: Backup):
    """ Create remote backup file name from local backup file information """
    return (local_file.name + '_' + local_file.slug).replace(" ", "-").replace(":", "_")


def intersect_files(local_files: dict[str, Backup], remote_files: [ResourceObject]) -> [BackupFile]:
    """ Intersect information from local and remote backup files """
    fmt = "%Y-%m-%d %H:%M:%S %Z"

    result = []
    remote_file_processed = set()

    remote_file_names = set([ro.name for ro in remote_files])

    for backup in local_files.values():
        remote_name = get_remote_file_name_from_local(backup)

        result_file = BackupFile(backup.slug, backup.name, backup.date.strftime(fmt), str(backup.size), True)
        if remote_name in remote_file_names:
            result_file.in_remote = True
            remote_file_processed.add(remote_name)
        result.append(result_file)

    for remote_file in remote_files:
        if remote_file.name in remote_file_processed:
            continue
        result_file = BackupFile('', remote_file.name, remote_file.modified.strftime(fmt), str(byte_to_mb(remote_file.size)), False, True)
        result.append(result_file)

    result.sort(key=lambda b: b.date, reverse=True)
    return result


class YaDsk:
    """ Core integration class.
    Contains all method for YandexDisk communication.
    """

    _token = None
    _refresh_token_value = None
    _token_expire_date = None
    _max_remote_file_amount = 10
    _is_schedule_loaded = False
    _schedule = None

    def __init__(self, logger):
        options = read_options()
        self._client_id = options[OPTION_CLIENT_ID]
        self._client_secret = options[OPTION_CLIENT_SECRET]
        self._path = options[OPTION_YD_PATH]
        self._schedule = options[OPTION_SCHEDULE]
        self._max_remote_file_amount = options[OPTION_REMOTE_MAX_QUANTITY]


        self._LOGGER = logger
        self._LOGGER.info('Create YaDsk')
        self._LOGGER.info('options: %s ', str(options))

        self._backup_observer = BackupObserver(logger, BACKUP_PATH)

    def load_token(self):
        """Load token info from file"""

        token_info = read_token()
        self._LOGGER.debug("token_info %s", str(token_info))
        if token_info is not None:
            self._token = token_info[CONF_TOKEN]
            self._refresh_token_value = token_info[CONF_REFRESH_TOKEN]
            self._token_expire_date = datetime.datetime.fromisoformat(token_info.get(CONF_TOKEN_EXPIRES,
                                                                                     datetime.datetime.now().isoformat()))
        else:
            self._token = None
            self._refresh_token_value = None
            self._token_expire_date = None

        self._LOGGER.debug("token_info %s", self._token)
        self._LOGGER.debug("token_info %s", self._refresh_token_value)
        self._LOGGER.debug("token_info %s", self._token_expire_date)

    def is_token_exists(self):
        """Check is token exists"""
        return self._token is not None

    def ensure_token(self):
        """ Check if token exist and try load token from file if not"""
        if not self.is_token_exists():
            self.load_token()
        return self.is_token_exists()

    def load_schedule(self):
        """ Load schedule setting and create cron task """
        if self._is_schedule_loaded:
            return

        self._LOGGER.debug("Create cron task")
        cron = CronTab(user=True)

        # Remove old task
        for job in cron:
            if job.comment == 'upload':
                cron.remove(job)
        cron.write()

        # Write new task
        job = cron.new(command='curl localhost:8099/upload1', comment='upload')
        job.setall(self._schedule)
        cron.write()

        self._is_schedule_loaded = True

    def getCheckCodeUrl(self):
        """ Get url for get check code from Yandex """
        return URL_GET_CODE + self._client_id

    def create_token(self, check_code):
        """ Get token from Yandex """
        try:
            self._LOGGER.debug("Try get token from YaDisk")
            yd = YaDisk(self._client_id, self._client_secret, '')
            token_object: TokenObject = yd.get_token(check_code)

            self._save_token(token_object)

        except Exception as e:
            self._LOGGER.error("Error when create token", exc_info=True)
            raise e

    def _save_token(self, token_object: TokenObject):
        """ Save token information into file """
        try:
            self._LOGGER.debug("Try save token")

            expire_seconds = token_object['expires_in']
            expire_data = datetime.datetime.now() + datetime.timedelta(seconds=expire_seconds)
            self._LOGGER.debug("New token expires in %s", expire_data)

            self._token = token_object['access_token']
            self._refresh_token_value = token_object['refresh_token']
            self._token_expire_date = expire_data

            token_info = {CONF_TOKEN: token_object['access_token'],
                          CONF_REFRESH_TOKEN: token_object['refresh_token'],
                          CONF_TOKEN_EXPIRES: expire_data.isoformat()}

            dump_token(token_info)
        except Exception as e:
            self._LOGGER.error("Error when create token", exc_info=True)
            raise e

    def get_files_info(self):
        """ Get data about backup files"""
        remote_files = self._list_yandex_disk()
        local_files = self._backup_observer.get_backups()

        return intersect_files(local_files, remote_files)

    def _list_yandex_disk(self) -> [ResourceObject]:
        """ List yandex disk directory.
        """
        if not self.is_token_exists():
            return []
        try:
            y = yadisk.YaDisk(token=self._token)
            files = self._file_list_processing(y.listdir(self._path))
            self._LOGGER.debug("Count files result: %s", len(files))
            return files
        except Exception as e:
            self._LOGGER.error("Error get directory info. Path: %s", self._path, exc_info=True)

    def _refresh_token_request(self, refresh_token, client_id, client_secret):
        """ Refresh token. """
        try:
            self._LOGGER.debug("Try refresh token")
            yd = YaDisk(client_id, client_secret, '')
            token_object: TokenObject = yd.refresh_token(refresh_token)

            self._save_token(token_object)

        except Exception as e:
            self._LOGGER.error("Error when refresh token", exc_info=True)
            raise e

    def _refresh_token_if_need(self, delta: datetime.timedelta):
        """ Refresh token when token lifetime go out bound """
        if (datetime.datetime.now() + delta) > self._token_expire_date:
            self._LOGGER.debug("Need refresh token")
            self._refresh_token_request(self._refresh_token_value, self._client_id, self._client_secret)
        else:
            self._LOGGER.debug("Refresh token not needed")

    def upload_files(self):
        """ Upload files to yandex.

            Upload new files and delete old files from yandex disk.
            Refresh yandex disk directory information
        """
        self._refresh_token_if_need(REFRESH_TOKEN_DELTA)

        local_backups = self.get_local_file_path_list(self._backup_observer.get_backups())
        remote_file_names = [x.name for x in self._list_yandex_disk()]
        remote_file_amount = len(remote_file_names)

        new_files = [file for file in local_backups.keys() if file not in remote_file_names]

        self._LOGGER.info("Need backup %d files", len(new_files))

        y = yadisk.YaDisk(token=self._token)
        for file in new_files:
            self._upload_file(y, str(local_backups[file]), self._path + '/' + file)
        is_deleted = False
        # Delete files from remote directory
        if (remote_file_amount + len(new_files)) > self._max_remote_file_amount:
            is_deleted = True
            old_file_count = (remote_file_amount + len(new_files)) - self._max_remote_file_amount
            if old_file_count > remote_file_amount:
                old_file_count = remote_file_amount

            self._LOGGER.debug("Need delete %d files", old_file_count)

            old_files = remote_file_names[-old_file_count:]

            for old_file in old_files:
                self._remove_file(y, self._path + '/' + old_file)

        return new_files or is_deleted

    def get_local_file_path_list(self, backups: dict[str, Backup]):
        """ Convert name home assistant backups to remote backups"""

        result = {}
        for backup in backups.values():
            key = get_remote_file_name_from_local(backup)
            result[key] = backup.path

        return result

    def _upload_file(self, y: yadisk.YaDisk, source_file, destination_file):
        """ Upload files to yandex disk """
        try:
            self._LOGGER.info('Upload file %s to %s', source_file, destination_file)
            y.upload(source_file, destination_file, overwrite=True, n_retries=3, retry_interval=5,
                     timeout=(15.0, 250.0))
            self._LOGGER.info('File %s uploaded to %s', source_file, destination_file)
        except Exception as e:
            self._LOGGER.error("Error upload file %s", source_file, exc_info=True)
            raise e

    def _remove_file(self, y: yadisk.YaDisk, deleted_file):
        """ Remove files from yandex disk """
        try:
            self._LOGGER.info('Remove file %s', deleted_file)
            y.remove(deleted_file, n_retries=3, retry_interval=5)
            self._LOGGER.info('File %s removed', deleted_file)
        except Exception as e:
            self._LOGGER.error("Error when remove file %s", deleted_file, exc_info=True)
            raise e

    @staticmethod
    def _file_list_processing(objects: Generator[any, any, ResourceObject]) -> list:
        """ Processing file list, received from Yandex Disk """
        files = [obj for obj in objects if obj.type == TYPE_FILE]
        result = sorted(files, key=lambda obj: obj.modified, reverse=True)
        return result
