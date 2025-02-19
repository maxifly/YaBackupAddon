# Changelog
## 2.01.2
**Change config** (add configuration parameter. May be need full reinstall).

- Add get information from network storage.  [#20](https://github.com/maxifly/YaBackupAddon/issues/20)
- Add upload from network storage.  [#25](https://github.com/maxifly/YaBackupAddon/issues/25)
- Add key icon for protected backup.  [#24](https://github.com/maxifly/YaBackupAddon/issues/24)

## 2.01.1
- Old backup deleted permanently. Trashcan not used.  [#21](https://github.com/maxifly/YaBackupAddon/issues/21)
- Fix uploaded time.  [#18](https://github.com/maxifly/YaBackupAddon/issues/18)

## 2.01.0
- Use bootstrap 5
- Add modal window with archive content information
- Remove deprecated "startup before" option  [#17](https://github.com/maxifly/YaBackupAddon/issues/17)


## 2.0.5
- Restore state entity after HA restart (by cron task) [#15](https://github.com/maxifly/YaBackupAddon/issues/15)

## 2.0.4
- Restore state entity after addon restart [#14](https://github.com/maxifly/YaBackupAddon/issues/14)

## 2.0.4-b
- Add entity sensor.yandex_backup_state
- Refactoring

## 2.0.3
- Up upload-big-file library version [#12](https://github.com/maxifly/YaBackupAddon/issues/12)

## 2.0.2
- Use prebuild docker image. [#10](https://github.com/maxifly/YaBackupAddon/issues/10)

## 2.0.1
- Add dark theme. Change config (add configuration parameter. May be need full reinstall). [#8](https://github.com/maxifly/YaBackupAddon/issues/8)

- Change readme. [#9](https://github.com/maxifly/YaBackupAddon/issues/9) 
## 2.0.0
Rewrite addon from python to go
You must get new token
## 1.2.4
Improve exception processing
## 1.2.3
Add download log methods
## 1.2.2
Use internal Flask wsgi server, because uWSGI not work properly with haos
## 1.2.1
Improve crontab initialization
## 1.2.0
Use uwsgi
Add cron.log
## 1.0.1
First version
