package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

var db *sql.DB

// Battle aka arena
type Battle struct {
	BattleID     string     `json:"id"`
	LeaderID     string     `json:"leaderId"`
	BattleName   string     `json:"name"`
	Warriors     []*Warrior `json:"warriors"`
	Plans        []*Plan    `json:"plans"`
	VotingLocked bool       `json:"votingLocked"`
	ActivePlanID string     `json:"activePlanId"`
}

// Warrior aka user
type Warrior struct {
	WarriorID   string `json:"id"`
	WarriorName string `json:"name"`
}

// Vote structure
type Vote struct {
	WarriorID string `json:"warriorId"`
	VoteValue string `json:"vote"`
}

// Plan aka Story structure
type Plan struct {
	PlanID     string  `json:"id"`
	PlanName   string  `json:"name"`
	Votes      []*Vote `json:"votes"`
	Points     string  `json:"points"`
	PlanActive bool    `json:"active"`
}

func SetupDB() {
	var (
		host     = GetEnv("DB_HOST", "db")
		port     = GetIntEnv("DB_PORT", 5432)
		user     = GetEnv("DB_USER", "thor")
		password = GetEnv("DB_PASS", "odinson")
		dbname   = GetEnv("DB_NAME", "thunderdome")
	)

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	var err error
	db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		log.Fatal("error connecting to the database: ", err)
	}

	if _, err := db.Exec(
		"CREATE TABLE IF NOT EXISTS battles (id UUID NOT NULL PRIMARY KEY, leader_id UUID, name VARCHAR(256), voting_locked BOOL DEFAULT true, active_plan_id UUID)"); err != nil {
		log.Fatal(err)
	}

	if _, err := db.Exec(
		"CREATE TABLE IF NOT EXISTS warriors (id UUID NOT NULL PRIMARY KEY, name VARCHAR(64))"); err != nil {
		log.Fatal(err)
	}

	if _, err := db.Exec(
		"CREATE TABLE IF NOT EXISTS plans (id UUID NOT NULL PRIMARY KEY, name VARCHAR(256), points VARCHAR(3) DEFAULT '', active BOOL DEFAULT false, battle_id UUID references battles(id) NOT NULL, votes JSONB DEFAULT '[]'::JSONB)"); err != nil {
		log.Fatal(err)
	}

	if _, err := db.Exec(
		"CREATE TABLE IF NOT EXISTS battles_warriors (battle_id UUID references battles NOT NULL, warrior_id UUID REFERENCES warriors NOT NULL, active BOOL DEFAULT false, PRIMARY KEY (battle_id, warrior_id))"); err != nil {
		log.Fatal(err)
	}
}

//CreateBattle adds a new battle to the map
func CreateBattle(LeaderID string, BattleName string) (*Battle, error) {
	newID, _ := uuid.NewUUID()
	id := newID.String()

	var b = &Battle{
		BattleID:     id,
		LeaderID:     LeaderID,
		BattleName:   BattleName,
		Warriors:     make([]*Warrior, 0),
		Plans:        make([]*Plan, 0),
		VotingLocked: true,
		ActivePlanID: ""}

	e := db.QueryRow(`INSERT INTO battles (id, leader_id, name) VALUES ($1, $2, $3) RETURNING id`, id, LeaderID, BattleName).Scan(&b.BattleID)
	if e != nil {
		log.Println(e)
		return nil, errors.New("Error Creating Battle")
	}

	return b, nil
}

// GetBattle gets a battle from the map by ID
func GetBattle(BattleID string) (*Battle, error) {
	var b = &Battle{
		BattleID:     BattleID,
		LeaderID:     "",
		BattleName:   "",
		Warriors:     make([]*Warrior, 0),
		Plans:        make([]*Plan, 0),
		VotingLocked: true,
		ActivePlanID: ""}

	// get battle
	var activePlanId sql.NullString
	e := db.QueryRow("SELECT id, name, leader_id, voting_locked, active_plan_id FROM battles WHERE id = $1", BattleID).Scan(&b.BattleID, &b.BattleName, &b.LeaderID, &b.VotingLocked, &activePlanId)
	if e != nil {
		log.Println(e)
		return nil, errors.New("Not found")
	}

	b.ActivePlanID = activePlanId.String
	b.Warriors = GetActiveWarriors(BattleID)
	b.Plans = GetPlans(BattleID)

	return b, nil
}

// CreateWarrior adds a new warrior to the db
func CreateWarrior(WarriorName string) *Warrior {
	newID, _ := uuid.NewUUID()
	id := newID.String()

	var WarriorID string
	e := db.QueryRow(`INSERT INTO warriors (id, name) VALUES ($1, $2) RETURNING id`, id, WarriorName).Scan(&WarriorID)
	if e != nil {
		log.Println(e)
	}

	return &Warrior{WarriorID: WarriorID, WarriorName: WarriorName}
}

// GetWarrior gets a warrior from db by ID
func GetWarrior(WarriorID string) (*Warrior, error) {
	var w Warrior

	e := db.QueryRow("SELECT id, name FROM warriors WHERE id = $1", WarriorID).Scan(&w.WarriorID, &w.WarriorName)
	if e != nil {
		log.Println(e)
		return nil, errors.New("Not found")
	}

	return &w, nil
}

// GetActiveWarriors retrieves the active warriors for a given battle from db
func GetActiveWarriors(BattleID string) []*Warrior {
	var warriors = make([]*Warrior, 0)
	rows, err := db.Query("SELECT warriors.id, warriors.name FROM battles_warriors LEFT JOIN warriors ON battles_warriors.warrior_id = warriors.id where battles_warriors.battle_id = $1 AND battles_warriors.active = true", BattleID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var w Warrior
			if err := rows.Scan(&w.WarriorID, &w.WarriorName); err != nil {
				log.Println(err)
			} else {
				warriors = append(warriors, &w)
			}
		}
	}

	return warriors
}

// AddWarriorToBattle adds a warrior by ID to the battle by ID
func AddWarriorToBattle(BattleID string, WarriorID string) ([]*Warrior, error) {
	if _, err := db.Exec(
		`INSERT INTO battles_warriors (battle_id, warrior_id, active) VALUES ($1, $2, true) ON CONFLICT (battle_id, warrior_id) DO UPDATE SET active = true`, BattleID, WarriorID); err != nil {
		log.Println(err)
	}

	warriors := GetActiveWarriors(BattleID)

	return warriors, nil
}

// RetreatWarrior removes a warrior from the current battle by ID
func RetreatWarrior(BattleID string, WarriorID string) []*Warrior {
	if _, err := db.Exec(
		`UPDATE battles_warriors SET active = false WHERE battle_id = $1 AND warrior_id = $2`, BattleID, WarriorID); err != nil {
		log.Println(err)
	}

	warriors := GetActiveWarriors(BattleID)

	return warriors
}

// GetPlans retrieves plans for given battle from db
func GetPlans(BattleID string) []*Plan {
	var plans = make([]*Plan, 0)
	planRows, plansErr := db.Query("SELECT id, name, points, active, votes FROM plans WHERE battle_id = $1", BattleID)
	if plansErr == nil {
		defer planRows.Close()
		for planRows.Next() {
			var v string
			var p = &Plan{PlanID: "",
				PlanName:   "",
				Votes:      make([]*Vote, 0),
				Points:     "",
				PlanActive: false}
			if err := planRows.Scan(&p.PlanID, &p.PlanName, &p.Points, &p.PlanActive, &v); err != nil {
				log.Println(err)
			} else {
				err = json.Unmarshal([]byte(v), &p.Votes)
				if err != nil {
					log.Println(err)
				}

				for i := range p.Votes {
					vote := p.Votes[i]
					if p.PlanActive {
						vote.VoteValue = ""
					}
				}

				plans = append(plans, p)
			}
		}
	}

	return plans
}

// CreatePlan adds a new plan to a battle
func CreatePlan(BattleID string, PlanName string) []*Plan {
	newID, _ := uuid.NewUUID()
	id := newID.String()

	var PlanID string
	e := db.QueryRow(`INSERT INTO plans (id, battle_id, name) VALUES ($1, $2, $3) RETURNING id`, id, BattleID, PlanName).Scan(&PlanID)
	if e != nil {
		log.Println(e)
	}

	plans := GetPlans(BattleID)

	return plans
}

// ActivatePlanVoting sets the plan by ID to active, wipes any previous votes/points, and disables votingLock
func ActivatePlanVoting(BattleID string, PlanID string) []*Plan {
	// set current to false
	if _, err := db.Exec(`UPDATE plans SET active = false WHERE battle_id = $1`, BattleID); err != nil {
		log.Println(err)
	}

	// set PlanID to true
	if _, err := db.Exec(
		`UPDATE plans SET active = true, points = '', votes = '[]'::jsonb WHERE id = $1`, PlanID); err != nil {
		log.Println(err)
	}

	// set battle VotingLocked and ActivePlanID
	if _, err := db.Exec(
		`UPDATE battles SET voting_locked = false, active_plan_id = $1 WHERE id = $2`, PlanID, BattleID); err != nil {
		log.Println(err)
	}

	plans := GetPlans(BattleID)

	return plans
}

// SetVote sets a warriors vote for the plan
func SetVote(BattleID string, WarriorID string, PlanID string, VoteValue string) []*Plan {
	// get plan
	var v string
	e := db.QueryRow("SELECT votes FROM plans WHERE id = $1", PlanID).Scan(&v)
	if e != nil {
		log.Println(e)
		// return nil, errors.New("Plan Not found")
	}
	var votes []*Vote
	err := json.Unmarshal([]byte(v), &votes)
	if err != nil {
		log.Println(err)
	}

	var voteIndex int
	var voteFound bool

	// find vote index
	for vi := range votes {
		if votes[vi].WarriorID == WarriorID {
			voteFound = true
			voteIndex = vi
			break
		}
	}

	if voteFound {
		votes[voteIndex].VoteValue = VoteValue
	} else {
		newVote := &Vote{WarriorID: WarriorID,
			VoteValue: VoteValue}

		votes = append(votes, newVote)
	}

	// update votes on Plan
	var votesJSON, _ = json.Marshal(votes)
	if _, err := db.Exec(
		`UPDATE plans SET votes = $1 WHERE id = $2`, string(votesJSON), PlanID); err != nil {
		log.Println(err)
	}

	plans := GetPlans(BattleID)

	return plans
}

// EndPlanVoting sets plan to active: false
func EndPlanVoting(BattleID string, PlanID string) []*Plan {
	// set current to false
	if _, err := db.Exec(`UPDATE plans SET active = false WHERE battle_id = $1`, BattleID); err != nil {
		log.Println(err)
	}

	// set battle VotingLocked
	if _, err := db.Exec(
		`UPDATE battles SET voting_locked = true WHERE id = $1`, BattleID); err != nil {
		log.Println(err)
	}

	plans := GetPlans(BattleID)

	return plans
}

// RevisePlanName updates the plan name by ID
func RevisePlanName(BattleID string, PlanID string, PlanName string) []*Plan {
	// set PlanID to true
	if _, err := db.Exec(
		`UPDATE plans SET name = $1 WHERE id = $2`, PlanName, PlanID); err != nil {
		log.Println(err)
	}

	plans := GetPlans(BattleID)

	return plans
}

// BurnPlan removes a plan from the current battle by ID
func BurnPlan(BattleID string, PlanID string) []*Plan {
	var isActivePlan bool

	// get plan
	e := db.QueryRow("DELETE FROM plans WHERE id = $1 RETURNING active", PlanID).Scan(&isActivePlan)
	if e != nil {
		log.Println(e)
		// return nil, errors.New("Plan Not found")
	}

	if isActivePlan {
		if _, err := db.Exec(
			`UPDATE battles SET voting_locked = true, active_plan_id = null WHERE id = $1`, BattleID); err != nil {
			log.Println(err)
		}
	}

	plans := GetPlans(BattleID)

	return plans
}

// FinalizePlan sets plan to active: false
func FinalizePlan(BattleID string, PlanID string, PlanPoints string) []*Plan {
	// set PlanID to true
	if _, err := db.Exec(
		`UPDATE plans SET active = false, points = $1 WHERE id = $2`, PlanPoints, PlanID); err != nil {
		log.Println(err)
	}

	// set battle ActivePlanID
	if _, err := db.Exec(
		`UPDATE battles SET active_plan_id = null WHERE id = $1`, BattleID); err != nil {
		log.Println(err)
	}

	plans := GetPlans(BattleID)

	return plans
}