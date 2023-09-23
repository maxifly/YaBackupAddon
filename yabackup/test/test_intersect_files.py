import datetime
import unittest
from pathlib import Path

from yadisk.objects import ResourceObject

from yabackup.src.root.bkp_observer import Backup
from yabackup.src.root.yad import intersect_files


class SquareEqSolverTestCase(unittest.TestCase):
    def test_intersect_empty(self):
        result = intersect_files(dict(), [])
        self.assertEqual(0, len(result))

    def test_intersect_only_local(self):
        local = self.get_local()
        result = intersect_files(local, [])
        self.assertEqual(3, len(result))

        without_remote = [r.name for r in result if not r.in_remote]
        self.assertEqual(3, len(without_remote))

    def test_intersect_only_remote(self):
        remote = self.get_remote()
        result = intersect_files(dict(), remote)
        self.assertEqual(3, len(result))

        with_remote = [r.name for r in result if r.in_remote]
        self.assertEqual(3, len(with_remote))

    def test_local_and_remote(self):
        local = self.get_local()
        remote = self.get_remote()
        result = intersect_files(local, remote)
        self.assertEqual(4, len(result))

        self.assertEqual("filename 3", result[0].name)
        self.assertEqual("filename 2", result[1].name)
        self.assertEqual("filename 1", result[2].name)
        self.assertEqual("filename-4", result[3].name)

        self.assertTrue(result[0].in_local)
        self.assertTrue(result[1].in_local)
        self.assertTrue(result[2].in_local)
        self.assertFalse(result[3].in_local)

        self.assertFalse(result[0].in_remote)
        self.assertTrue(result[1].in_remote)
        self.assertTrue(result[2].in_remote)
        self.assertTrue(result[3].in_remote)

    def get_local(self):
        local = dict()
        local['slug1'] = Backup(
            slug='slug1',
            name='filename 1',
            date=datetime.datetime.now(),
            path=Path("/file1.tar"),
            size=123.0,
        )
        local['slug2'] = Backup(
            slug='slug2',
            name='filename 2',
            date=datetime.datetime.now() + datetime.timedelta(days=1),
            path=Path("/file2.tar"),
            size=123.0,
        )
        local['slug3'] = Backup(
            slug='slug3',
            name='filename 3',
            date=datetime.datetime.now() + datetime.timedelta(days=2),
            path=Path("/file3.tar"),
            size=123.0,
        )
        return local

    def get_remote(self):
        remote = []

        datetime.datetime.strptime('2020-01-30T20:59:59+0000', "%Y-%m-%dT%H:%M:%S%z")

        remote.append(ResourceObject({'name': 'filename-1_slug1', 'size': 1024, 'modified': '2020-01-30T21:59:59+00:00'}))
        remote.append(ResourceObject({'name': 'filename-2_slug2', 'size': 1024, 'modified': '2020-01-30T20:59:59+00:00'}))
        remote.append(ResourceObject({'name': 'filename-4', 'size': 1024, 'modified': '2020-01-30T20:58:59+00:00'}))
        return remote
