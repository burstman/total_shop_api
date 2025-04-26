package console

import (
	"convertyApi/service"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/manifoldco/promptui"
	"github.com/olekukonko/tablewriter"
)

// Run starts the console interface
func Run(dataService service.DataService) {
	for {
		prompt := promptui.Select{
			Label: "Select Action",
			Items: []string{
				"List All Records",
				"List Issues",
				"List Orders",
				"Query by ID",
				"Insert New Record",
				"Exit",
			},
		}

		_, result, err := prompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed: %v\n", err)
			return
		}

		switch result {
		case "List All Records":
			listRecords(dataService)
		case "List Issues":
			listIssues(dataService)
		case "List Orders":
			listOrders(dataService)
		case "Query by ID":
			queryByID(dataService)
		case "Insert New Record":
			insertRecord(dataService)
		case "Exit":
			fmt.Println("Exiting...")
			return
		}
	}
}

func listRecords(dataService service.DataService) {
	records, err := dataService.ListRecords()
	if err != nil {
		fmt.Printf("Error fetching records: %v\n", err)
		return
	}
	if len(records) == 0 {
		fmt.Println("No records found in the database")
		return
	}

	// Create table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "UserID", "Type", "Details", "Status", "CreatedAt"})
	table.SetBorder(true)
	table.SetAutoWrapText(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetColumnSeparator("|")
	table.SetCenterSeparator("+")
	table.SetRowSeparator("-")

	for _, record := range records {
		var detailsMap map[string]interface{}
		if err := json.Unmarshal(record.Details, &detailsMap); err != nil {
			fmt.Printf("Error unmarshaling details for record %d: %v\n", record.ID, err)
			continue
		}
		details, err := json.Marshal(detailsMap)
		if err != nil {
			fmt.Printf("Error marshaling details for record %d: %v\n", record.ID, err)
			continue
		}
		detailsStr := string(details)
		if len(detailsStr) > 50 {
			detailsStr = detailsStr[:47] + "..."
		}
		createdAtStr := record.CreatedAt.Format("2006-01-02 15:04:05")
		table.Append([]string{
			fmt.Sprintf("%d", record.ID),
			fmt.Sprintf("%d", record.UserID),
			record.Type,
			detailsStr,
			record.Status,
			createdAtStr,
		})
	}

	fmt.Println("\nRecords from chatbot.interactions:")
	table.Render()
}

func listIssues(dataService service.DataService) {
	issues, err := dataService.ListIssues()
	if err != nil {
		fmt.Printf("Error fetching issues: %v\n", err)
		return
	}
	if len(issues) == 0 {
		fmt.Println("No issues found in the database")
		return
	}

	// Create table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Type", "Name", "Product", "Description", "Phone Number", "Status", "CreatedAt"})
	table.SetBorder(true)
	table.SetAutoWrapText(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetColumnSeparator("|")
	table.SetCenterSeparator("+")
	table.SetRowSeparator("-")
	table.SetColWidth(80)

	for _, issue := range issues {
		var detailsMap map[string]interface{}
		if err := json.Unmarshal(issue.Details, &detailsMap); err != nil {
			fmt.Printf("Error unmarshaling details for issue: %v\n", err)
			continue
		}
		issueType := fmt.Sprintf("%v", detailsMap["type"])
		name := fmt.Sprintf("%v", detailsMap["name"])
		product := fmt.Sprintf("%v", detailsMap["product"])
		description := fmt.Sprintf("%v", detailsMap["description"])
		phoneNumber := fmt.Sprintf("%v", detailsMap["phone_number"])
		detailStatus := fmt.Sprintf("%v", detailsMap["status"])
		if len(description) > 60 {
			description = description[:57] + "..."
		}
		createdAtStr := issue.CreatedAt.Format("2006-01-02 15:04:05")
		table.Append([]string{
			issueType,
			name,
			product,
			description,
			phoneNumber,
			detailStatus,
			createdAtStr,
		})
	}

	fmt.Println("\nIssues from chatbot.interactions:")
	table.Render()
}

func listOrders(dataService service.DataService) {
	// Prompt for query parameters
	query := service.CustomerOrderQuery{}

	pagePrompt := promptui.Prompt{
		Label:   "Enter Page (default 1)",
		Default: "1",
	}
	pageStr, err := pagePrompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed: %v\n", err)
		return
	}
	if pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil {
			fmt.Println("Invalid page number")
			return
		}
		query.Page = page
	} else {
		query.Page = 1
	}

	limitPrompt := promptui.Prompt{
		Label:   "Enter Limit (default 10)",
		Default: "10",
	}
	limitStr, err := limitPrompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed: %v\n", err)
		return
	}
	if limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			fmt.Println("Invalid limit number")
			return
		}
		query.Limit = limit
	} else {
		query.Limit = 10
	}

	statusPrompt := promptui.Prompt{
		Label: "Enter Status (e.g., pending, shipped, optional)",
	}
	status, err := statusPrompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed: %v\n", err)
		return
	}
	query.Status = status

	// Add more prompts as needed (e.g., archived, search)
	// For simplicity, set defaults
	archived := false
	query.Archived = &archived

	orders, err := dataService.ListOrders(query)
	if err != nil {
		fmt.Printf("Error fetching orders: %v\n", err)
		return
	}
	if len(orders) == 0 {
		fmt.Println("No orders found")
		return
	}

	// Create table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Name", "Address", "Note", "Email", "Phone", "City", "Status", "CreatedAt"})
	table.SetBorder(true)
	table.SetAutoWrapText(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetColumnSeparator("|")
	table.SetCenterSeparator("+")
	table.SetRowSeparator("-")
	table.SetColWidth(80)

	for _, order := range orders {
		address := order.Customer.Address
		if len(address) > 60 {
			address = address[:57] + "..."
		}
		createdAtStr := order.CreatedAt.Format("2006-01-02 15:04:05")
		table.Append([]string{
			order.ID,
			order.Customer.Name,
			address,
			order.Customer.Note,
			order.Customer.Email,
			order.Customer.Phone,
			order.Customer.City,
			order.Status,
			createdAtStr,
		})
	}

	fmt.Println("\nOrders from Converty.shop:")
	table.Render()
}

func queryByID(dataService service.DataService) {
	prompt := promptui.Prompt{
		Label: "Enter Record ID",
	}

	idStr, err := prompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed: %v\n", err)
		return
	}

	var id uint
	_, err = fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		fmt.Println("Invalid ID format")
		return
	}

	record, err := dataService.QueryByID(id)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var detailsMap map[string]interface{}
	if err := json.Unmarshal(record.Details, &detailsMap); err != nil {
		fmt.Printf("Error unmarshaling details: %v\n", err)
		return
	}
	details, err := json.MarshalIndent(detailsMap, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling details: %v\n", err)
		return
	}
	fmt.Printf("ID: %d\nUserID: %d\nType: %s\nDetails: %s\nStatus: %s\nCreatedAt: %s\n",
		record.ID, record.UserID, record.Type, details, record.Status, record.CreatedAt)
}

func insertRecord(dataService service.DataService) {
	userIDPrompt := promptui.Prompt{
		Label: "Enter User ID",
	}
	userIDStr, err := userIDPrompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed: %v\n", err)
		return
	}

	var userID uint
	_, err = fmt.Sscanf(userIDStr, "%d", &userID)
	if err != nil {
		fmt.Println("Invalid User ID format")
		return
	}

	typePrompt := promptui.Prompt{
		Label: "Enter Table Type (address/order/issue)",
	}
	tableType, err := typePrompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed: %v\n", err)
		return
	}

	var details map[string]interface{}
	if tableType == "issue" {
		issueTypePrompt := promptui.Prompt{
			Label: "Enter Issue Type (e.g., defective, delivery)",
		}
		issueType, err := issueTypePrompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed: %v\n", err)
			return
		}

		namePrompt := promptui.Prompt{
			Label: "Enter Name",
		}
		name, err := namePrompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed: %v\n", err)
			return
		}

		productPrompt := promptui.Prompt{
			Label: "Enter Product",
		}
		product, err := productPrompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed: %v\n", err)
			return
		}

		descPrompt := promptui.Prompt{
			Label: "Enter Description",
		}
		description, err := descPrompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed: %v\n", err)
			return
		}

		phonePrompt := promptui.Prompt{
			Label: "Enter Phone Number",
		}
		phoneNumber, err := phonePrompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed: %v\n", err)
			return
		}

		detailStatusPrompt := promptui.Prompt{
			Label: "Enter Detail Status (e.g., Pending, Resolved)",
		}
		detailStatus, err := detailStatusPrompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed: %v\n", err)
			return
		}

		details = map[string]interface{}{
			"type":         issueType,
			"name":         name,
			"product":      product,
			"description":  description,
			"phone_number": phoneNumber,
			"status":       detailStatus,
		}
	} else {
		detailsPrompt := promptui.Prompt{
			Label: "Enter JSON Details (e.g., {\"key\": \"value\"})",
		}
		detailsStr, err := detailsPrompt.Run()
		if err != nil {
			fmt.Printf("Prompt failed: %v\n", err)
			return
		}

		if err := json.Unmarshal([]byte(detailsStr), &details); err != nil {
			fmt.Printf("Invalid JSON format: %v\n", err)
			return
		}
	}

	statusPrompt := promptui.Prompt{
		Label: "Enter Table Status (pending/completed)",
	}
	tableStatus, err := statusPrompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed: %v\n", err)
		return
	}

	_, err = dataService.InsertRecord(userID, tableType, details, tableStatus)
	if err != nil {
		fmt.Printf("Error inserting record: %v\n", err)
		return
	}

	fmt.Println("Record created successfully!")
}
