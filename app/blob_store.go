package app

type BlobStore struct {}

type ActivityMetadata struct {}

func (bs *BlobStore) PersistGpx(AthleteId int, GpxData []byte) error {
	return nil
}

func (bs *BlobStore) SaveActivityMetadata(AthleteId int, activity ActivityMetadata) error {
	return nil
}

