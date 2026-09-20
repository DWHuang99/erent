package externalapikey

import "gorm.io/gorm"

type Repository struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

//TODO
func (r *Repository) Add() {

}

//TODO
func (r *Repository) GetUsersApiKey() {}

//TODO
func (r *Repository) QueryByApiKey() {

}

//TODO
func (r *Repository) Delete() {

}
