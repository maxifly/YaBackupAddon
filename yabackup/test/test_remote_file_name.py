import unittest
import datetime
from pathlib import Path

from yabackup.src.root.bkp_observer import Backup
from yabackup.src.root.yad import get_remote_file_name_from_local


class SquareEqSolverTestCase(unittest.TestCase):

    def test_remote_name(self):
        values = []
        values.append(('file1_slug1', Backup(
                slug='slug1',
                name='file1',
                date=str(datetime.datetime.now()),
                path=Path("/file1.tar"),
                size=123.0,
            )))

        values.append(('file-2_slug1', Backup(
                slug='slug1',
                name='file 2',
                date=str(datetime.datetime.now()),
                path=Path("/file1.tar"),
                size=123.0,
            )))


        values.append(('file-33_44_55_slug1', Backup(
                slug='slug1',
                name='file 33:44:55',
                date=str(datetime.datetime.now()),
                path=Path("/file1.tar"),
                size=123.0,
            )))


        for value in values:
          self.assertEqual(value[0],  get_remote_file_name_from_local(value[1]))


