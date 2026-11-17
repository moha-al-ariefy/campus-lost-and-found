# Campus Lost & Found Windows test

Team LAMNZ · RIT Dubai SWEN 261 final project

Allow about 10 minutes for the shared staging check. Use invented items and identifying clues for every test.

## Start

1. Extract the ZIP and open `CampusLostFound.exe`. If Windows asks for WebView2 Runtime, use the [Microsoft installer](https://developer.microsoft.com/en-us/microsoft-edge/webview2/).
2. Sign in with the staging test account Mohammed assigned you. Use **Create account** to test campus-user signup. Never use your university password.
3. If the app asks you to set up a local instance, you're using the offline build. Local records stay on that computer. Use the Azure build for the two-client check below.
4. After your first sign-in, follow the welcome guide or select **Skip**. You can reopen it with **How to use the app** at the bottom of the page.

## Check the shared workflow

1. As `reception`, select **Record an item**. Use a distinctive item name such as `TEST headphones 018`, general location `Library`, and a private clue such as `blue tape inside the case`. Add a test JPEG or PNG if you have one. Save and note the item reference.
2. On a second computer, sign in as a campus user. Search by the exact item name and open it. Confirm the reference matches. The private clue and photo should be hidden.
3. As that campus user, submit a claim with the matching clue. Open **Claims** and check that it says **Submitted**.
4. As `admin`, open the claim, compare the private evidence, and approve only after recording both verification checks. The campus user should see **Approved**. Use role accounts assigned for this test.
5. As `reception`, go to **Claims → Approved handovers**. For this test item, record verification and a fictional signed paper receipt reference. Check that the item leaves the directory and the claimant sees that collection was recorded.
6. Use Tab, Shift+Tab and Enter through sign-in, search and claim submission. Resize the window and note anything clipped or unreachable.

## Send back

Fill in [FEEDBACK.md](FEEDBACK.md) with PASS, FAIL or NOT RUN for each step. Include your Windows version, the item reference, and what you expected and saw. If the unsigned preview is blocked, report the Windows message. Leave passwords, university details and screenshots of private data out of the feedback.
