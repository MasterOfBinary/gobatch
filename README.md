GoBatch
=======

[![Build Status](https://travis-ci.org/MasterOfBinary/gobatch.svg?branch=master)](https://travis-ci.org/MasterOfBinary/gobatch)
[![Coverage Status](https://coveralls.io/repos/github/MasterOfBinary/gobatch/badge.svg?branch=master)](https://coveralls.io/github/MasterOfBinary/gobatch?branch=master)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/MasterOfBinary/gobatch)](https://pkg.go.dev/github.com/MasterOfBinary/gobatch)

GoBatch is a batch processing library for Go. The data reader and processor are
implementations of `batch.Source` and `batch.Processor`, respectively. The
actual data input and the processing of a batch of items is done by the
user, while the `batch.Batch` structure provided by the GoBatch library
handles the rest of the pipeline.

The batch pipeline consists of several stages:

1. Reading from the source, which could be a channel, disk, Redis, or pretty
much anywhere. All that's needed is a `Source` implementation that knows how to
do it.
2. The data is written to the channels provided by a `batch.PipelineStage` and
passed to `Batch`, which queues the items and prepares them for processing. It
decides how many items to batch together based on its configuration.
3. `Batch` sends the items in batches, as a `PipelineStage`, to the `Processor`,
which does whatever is necessary with the data.

Typical usage of GoBatch is when reading or writing to a remote database or Redis.
By batching together multiple calls, fewer connections are made and network traffic
is reduced.

NOTE: GoBatch is considered a version 0 release and is in an unstable state.
Compatibility may be broken at any time on the master branch. If you need a stable
release, wait for version 1.

Features
--------

* Complete control over the number of items to process at once and amount of time
between batches.
* Errors are returned over a channel, so that they can be logged or otherwise
handled.
* GoBatch is concurrency-safe.

Documentation
-------------

See the [pkg.go.dev docs](https://pkg.go.dev/github.com/MasterOfBinary/gobatch) for documentation
and an [example](https://pkg.go.dev/github.com/MasterOfBinary/gobatch/batch#example-package).

Installation
------------

To download, run

    go get github.com/MasterOfBinary/gobatch

GoBatch doesn't require any dependencies except Go 1.7 or later and the
standard library.

Examples
--------

See the [pkg.go.dev docs](https://pkg.go.dev/github.com/MasterOfBinary/gobatch) for examples.

License
-------

GoBatch is provided under the MIT licence. See LICENSE for more details.
