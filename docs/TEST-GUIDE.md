# Campus Lost & Found: quick test

Team LAMNZ · RIT Dubai SWEN 261 final project · Iteration 1

Allow about 10 minutes. These steps are for the local Windows preview, where each computer has its own records. Use invented test items only.

**To add a found item:** sign in as `reception` or `affairs` using the password you chose during setup. Select **Record an item** from the sidebar or the top of **Found items**. Fill in the category, description, general location, date found and received time, confirm the holding point, then select **Save found item**. The `student` account can search the directory.

## Start

1. Extract the complete ZIP into a folder, then open `CampusLostFound.exe`.
2. Windows needs Microsoft Edge WebView2 Runtime. If missing, install it from https://developer.microsoft.com/en-us/microsoft-edge/webview2/ . No Node or Go installation is needed.
3. This preview executable is unsigned. If Windows blocks it, report the exact message to Mohammed; do not disable security protection.
4. On the first launch, choose a test password of at least 12 characters. Do not use your university password. The four local test accounts below use the password you chose. If setup is absent, this computer already has a local database.

| Account | Role | What to check |
| --- | --- | --- |
| reception | Reception staff | Record an item at Reception; view its private clues |
| affairs | Student Affairs staff | Record an item at Student Affairs; other staff's clues remain private |
| student | Campus user | Search public details; no intake or ownership clues |
| admin | Administrator | Inspect private details and creation history |

## Try these checks

1. Enter an incorrect password. Expect an error, not a signed-in screen.
2. Sign in as `reception`. Select **Record an item**. Try saving with required fields missing; expect validation and no saved record.
3. Create an Electronics item, general location `Library - TEST`, description `TEST headphones`, and identifying detail `TEST blue tape inside case`. Use today's found date and a valid received time. If the build has an **Item photo** field, attach a test JPEG or PNG and check the preview. Keep clues out of the public location. Save once and record its reference.
4. Search by `Library - TEST`, category and holding point. Expect your item. Search for an unrelated location; expect no results. Clear filters; expect the item again.
5. Open the item as its creator. Expect the description, private clue and any attached photo. Close and reopen the app, sign in if needed, and find the same reference. Expect exactly one record.
6. Sign out and sign in as `student`. Find the same record. Expect only directory details. The private description, blue-tape clue, photo and intake action should be hidden.
7. Sign in as `affairs`. The reception item's clues should remain private. Any new intake must use the assigned Student Affairs holding point.
8. Sign in as `admin`. Expect the item's private details, any attached photo and its creation activity.
9. Use Tab, Shift+Tab, Enter and the arrow keys through sign-in, filters and intake. Resize the window. Report anything clipped, unreadable, slow or unreachable.

Report each check as PASS, FAIL or NOT RUN. Lost reports, claims and handover remain in development.

## Send back

Fill in `FEEDBACK.md` and send it to Mohammed with screenshots if useful. Include your Windows version, approximate RAM, the step, expected result and actual result. Do not send passwords, university details or your database. Do not put real lost-property data into this preview.

Data stays at `%APPDATA%\CampusLostFound\campus.db` after closing or replacing the executable. Keep it for restart checks. No password-reset or database-reset UI exists in this iteration; report access problems instead of deleting files.
