#!/usr/bin/env python3
"""
Test script to check if the API returns correct enable status for clients
"""

import requests
import json

# Configuration
BASE_URL = "http://localhost:2053"  # Adjust if your panel runs on different port
USERNAME = "admin"  # Replace with your username
PASSWORD = "admin"  # Replace with your password

def login():
    """Login and get session cookie"""
    login_url = f"{BASE_URL}/login"
    data = {
        "username": USERNAME,
        "password": PASSWORD
    }
    
    session = requests.Session()
    response = session.post(login_url, data=data)
    
    if response.status_code == 200:
        print(f"✓ Login successful")
        return session
    else:
        print(f"✗ Login failed: {response.status_code}")
        return None

def check_inbounds(session):
    """Fetch inbounds and check ClientStats enable field"""
    api_url = f"{BASE_URL}/panel/api/inbounds/list"
    
    response = session.get(api_url)
    
    if response.status_code != 200:
        print(f"✗ API request failed: {response.status_code}")
        return
    
    data = response.json()
    
    if not data.get("success"):
        print(f"✗ API returned error: {data}")
        return
    
    inbounds = data.get("obj", [])
    print(f"\n✓ Received {len(inbounds)} inbounds from API\n")
    
    # Check each inbound's clientStats
    for inbound in inbounds:
        inbound_id = inbound.get("id")
        remark = inbound.get("remark", "N/A")
        client_stats = inbound.get("clientStats", [])
        
        print(f"Inbound #{inbound_id} ({remark}):")
        print(f"  ClientStats count: {len(client_stats)}")
        
        if not client_stats:
            print(f"  (No ClientStats)")
            continue
        
        for stats in client_stats:
            email = stats.get("email")
            enable = stats.get("enable")
            account_id = stats.get("accountId", 0)
            
            status_icon = "✓" if enable else "✗"
            print(f"  {status_icon} {email}: enable={enable}, accountId={account_id}")
        
        print()

def main():
    print("=== Checking API Response for Client Enable Status ===\n")
    
    session = login()
    if not session:
        return
    
    check_inbounds(session)
    
    print("\n=== Check Complete ===")
    print("\nExpected: Clients with account_id=1 should have enable=False/0")
    print("If you see enable=True, there's a bug in the API serialization")

if __name__ == "__main__":
    main()
