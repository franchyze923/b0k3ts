package badger

import (
	"log"
	"log/slog"

	badgerDB "github.com/dgraph-io/badger/v4"
)

func InitializeDatabase() *badgerDB.DB {

	// Starting Badger
	//
	db, err := badgerDB.Open(badgerDB.DefaultOptions("/tmp/badger"))
	if err != nil {
		log.Fatal(err)
	}

	return db

}

func PullKV(db *badgerDB.DB, key string) ([]byte, error) {

	var valCopy []byte

	err := db.View(func(txn *badgerDB.Txn) error {
		// Get the item
		item, err := txn.Get([]byte(key))
		if err != nil {
			slog.Error(err.Error())
			return err
		}

		valCopy, err = item.ValueCopy(nil)
		if err != nil {
			slog.Error(err.Error())
			return err
		}

		return nil
	})
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	return valCopy, nil
}

func PutKV(db *badgerDB.DB, key string, value []byte) (err error) {

	err = db.Update(func(txn *badgerDB.Txn) error {
		// Keys and values must be byte slices
		err := txn.Set([]byte(key), value)
		if err != nil {
			slog.Error(err.Error())
			return err
		}

		return nil

	})
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	return nil
}
