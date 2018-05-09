package main

import (
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
)

// Check handles adding the proper interval for check.cgi along with future
// challenge solving and future mail existence checking.
// BUG(spotlightishere): Challenge solving isn't implemented whatsoever.
func Check(w http.ResponseWriter, r *http.Request, db *sql.DB, inter int) {
	mlchkidStmt, err := db.Prepare("SELECT `mlid` FROM accounts WHERE `mlchkid` = ?")
	if err != nil {
		fmt.Fprintf(w, GenNormalErrorCode(420, "Unable to formulate authentication statement."))
		LogError("Unable to prepare check statement", err)
		return
	}
	// Grab string of interval
	interval := strconv.Itoa(inter)
	// Add required headers
	w.Header().Add("Content-Type", "text/plain;charset=utf-8")
	w.Header().Add("X-Wii-Mail-Download-Span", interval)
	w.Header().Add("X-Wii-Mail-Check-Span", interval)

	// HMAC key most likely used for `chlng`
	// TODO: insert hmac thing
	// "ce4cf29a3d6be1c2619172b5cb298c8972d450ad" is the actual
	// hmac key, according to Larsenv.
	hmacKey := "ce4cf29a3d6be1c2619172b5cb298c8972d450ad"

	// Parse form in preparation for finding mail.
	err = r.ParseForm()
	if err != nil {
		fmt.Fprint(w, GenNormalErrorCode(320, "Unable to parse parameters."))
		LogError("Unable to parse form", err)
		return
	}

	mlchkid := r.Form.Get("mlchkid")
	if mlchkid == "" {
		fmt.Fprintf(w, GenNormalErrorCode(320, "Unable to parse parameters."))
		return
	}

	// Grab salt + mlchkid sha512
	hashByte := sha512.Sum512(append(salt, []byte(mlchkid)...))
	hash := hex.EncodeToString(hashByte[:])

	// Check mlchkid
	result, err := mlchkidStmt.Query(hash)
	if err != nil {
		fmt.Fprintf(w, GenNormalErrorCode(320, "Unable to parse parameters."))
		LogError("Unable to run mlchkid query", err)
		return
	}

	mlidStatement, err := db.Prepare("SELECT * FROM `mails` WHERE `recipient_id` =? AND `sent` = 0 ORDER BY `timestamp` ASC")
	if err != nil {
		LogError("Unable to prepare mlid statement", err)
	}

	// By default, we'll assume there's no mail.
	mailFlag := "0"
	resultsLoop := 0

	// Scan through returned rows.
	defer result.Close()
	for result.Next() {
		var mlid string
		err = result.Scan(&mlid)

		// Splice off w from mlid
		storedMail, err := mlidStatement.Query(mlid[1:])
		if err != nil {
			LogError("Unable to run mlid", err)
			return
		}

		size := 0
		defer storedMail.Close()
		for storedMail.Next() {
			size++
		}
		err = result.Err()
		if err != nil {
			fmt.Fprintf(w, GenNormalErrorCode(420, "Unable to formulate authentication statement."))
			LogError("Unable to get user mail", err)
			return
		}

		// Set mail flag to number of mail taken from database
		mailFlag = strconv.Itoa(size)
		resultsLoop++
	}

	err = result.Err()
	if err != nil {
		fmt.Fprintf(w, GenNormalErrorCode(420, "Unable to formulate authentication statement."))
		LogError("Generic database issue", err)
		return
	}

	if resultsLoop == 0 {
		// Looks like that user didn't exist.
		fmt.Fprintf(w, GenNormalErrorCode(220, "Invalid authentication."))
		return
	}

	// https://github.com/RiiConnect24/Mail-Go/wiki/check.cgi for response format
	fmt.Fprint(w, GenSuccessResponse(),
		"res=", hmacKey, "\n",
		"mail.flag=", mailFlag, "\n",
		"interval=", interval)
}
