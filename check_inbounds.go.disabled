package main

import (
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func main() {
	godotenv.Load()
	
	err := database.InitDB(config.GetDBPath())
	if err != nil {
		log.Fatal(err)
	}

	db := database.GetDB()
	
	// Check all inbounds
	var inbounds []*model.Inbound
	db.Find(&inbounds)
	
	fmt.Println("=== All Inbounds ===")
	for _, inbound := range inbounds {
		fmt.Printf("ID: %d, Remark: %s, Port: %d, Protocol: %s, SlaveID: %d, Enable: %v\n",
			inbound.Id, inbound.Remark, inbound.Port, inbound.Protocol, inbound.SlaveId, inbound.Enable)
	}
	
	// Check all slaves
	var slaves []*model.Slave
	db.Find(&slaves)
	
	fmt.Println("\n=== All Slaves ===")
	for _, slave := range slaves {
		fmt.Printf("ID: %d, Name: %s, Status: %s\n", slave.Id, slave.Name, slave.Status)
	}
	
	// Check inbounds for slave 1
	var slaveInbounds []*model.Inbound
	db.Where("slave_id = ?", 1).Find(&slaveInbounds)
	
	fmt.Println("\n=== Inbounds for Slave 1 ===")
	if len(slaveInbounds) == 0 {
		fmt.Println("No inbounds assigned to Slave 1")
	} else {
		for _, inbound := range slaveInbounds {
			fmt.Printf("ID: %d, Remark: %s, Port: %d, Protocol: %s, Enable: %v\n",
				inbound.Id, inbound.Remark, inbound.Port, inbound.Protocol, inbound.Enable)
		}
	}
}
