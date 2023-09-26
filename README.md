# Bug-Bounty-Go-Scrapper

This project is a Go-based scraper designed to fetch  
and compare bug bounty information from immunefi.com.   
Monitors and logs differences in bug bounties data.   
Sends notifications via Telegram for changes.   
Persists data in MongoDB for efficient comparisons.    

**Pre-requisites:**
Before running the scrapper, you'll need to set up your environment.

## Setup & Configuration
Clone the repository:


git clone https://github.com/0xMilenov/Bug-Bounty-Go-Scrapper.git     
cd Bug-Bounty-Go-Scrapper

## Environment Variables Setup:

You will need to create a .env file in the root directory with the following configuration:

```
# User Agent for Web Requests
USER_AGENT="YOUR_USER_AGENT_HERE"

# MongoDB Connection Details
MONGO_URI=YOUR_MONGO_URI_HERE
MONGO_DB=YOUR_DB_NAME_HERE

# Telegram Bot Details
TELEGRAM_BOT_TOKEN=YOUR_TELEGRAM_BOT_TOKEN_HERE
TELEGRAM_CHAT_ID=YOUR_TELEGRAM_CHAT_ID_HERE
```
Replace the placeholders with the appropriate values.     
For example, replace YOUR_USER_AGENT_HERE with your desired user agent.

Run the project:

```
go run main.go
```

## Notes:

Ensure you have the required permissions and configurations set up in MongoDB to connect and perform operations.
The Telegram bot token and chat ID should be obtained by setting up a Telegram bot and creating a channel or group where the bot can send messages.
