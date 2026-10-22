"""Build the shipped NER golden dataset.

Four files, and the reason for four is the whole point of this exercise:

  ner.json      — the DEVELOPMENT set. The decoder's rules were written
                  against it, so its numbers are optimistic by construction.
  holdout.json  — the HELD-OUT set. Written after the rules were fixed,
                  evaluated once, and the failures it produced were then
                  fixed.
  final.json    — the FINAL set. Written after the decoder was written,
                  evaluated once, and its failures were then fixed. Its
                  FIRST evaluation is the honest generalisation number.
  structured.json — the STRUCTURED-DOCUMENT set. Written after the corpora
                  showed the decoder reporting JSON keys, HTTP headers and
                  spreadsheet field labels as names. It is the regression
                  guard for the rules that fix that class.

All four are shipped so the gate catches regressions. The generalisation
figure is recorded in the gate, not re-derived from these files, because
these files are now seen data.
"""

import json
import os

DEV = [
    ("pos-ner-person-full", "A full personal name in a sentence.",
     "Marcus Feld signed the agreement on Tuesday.",
     [("PERSON", "Marcus Feld")]),
    ("pos-ner-person-first", "A single given name used as a subject.",
     "Priya will present the quarterly figures.",
     [("PERSON", "Priya")]),
    ("pos-ner-person-two", "Two people in one sentence, in text order.",
     "The handover went from Elena Duarte to Samuel Okoye.",
     [("PERSON", "Elena Duarte"), ("PERSON", "Samuel Okoye")]),
    ("pos-ner-person-three", "Three people in a list.",
     "Nadia Costa, Oliver Grant and Yusuf Demir attended the session.",
     [("PERSON", "Nadia Costa"), ("PERSON", "Oliver Grant"), ("PERSON", "Yusuf Demir")]),
    ("pos-ner-person-possessive", "A name in a possessive phrase.",
     "Ingrid Larsen's notes were circulated.",
     [("PERSON", "Ingrid Larsen")]),
    ("pos-ner-person-appositive", "A name followed by a role.",
     "Beatriz Nunez, the lead auditor, signed the page.",
     [("PERSON", "Beatriz Nunez")]),
    ("pos-ner-person-object", "A name as the object of a verb.",
     "The board appointed Halima Yusuf as chair.",
     [("PERSON", "Halima Yusuf")]),
    ("pos-ner-person-middle", "A middle name inside a full name.",
     "The witness was James Robert Ellery.",
     [("PERSON", "James Robert Ellery")]),
    ("pos-ner-person-after-email-word", "A name in a sentence that also contains a word that looks like a contact.",
     "Contact Amara Boateng for the shipment paperwork.",
     [("PERSON", "Amara Boateng")]),

    ("pos-ner-org-inc", "A company name with an Inc suffix.",
     "Fernwood Logistics Inc handles the freight.",
     [("ORGANIZATION", "Fernwood Logistics Inc")]),
    ("pos-ner-org-ltd", "A company name with an Ltd suffix.",
     "Brightwater Analytics Ltd published the report.",
     [("ORGANIZATION", "Brightwater Analytics Ltd")]),
    ("pos-ner-org-corp", "A company name with a Corp suffix.",
     "Halcyon Manufacturing Corp opened a new plant.",
     [("ORGANIZATION", "Halcyon Manufacturing Corp")]),
    ("pos-ner-org-gmbh", "A company name with a foreign suffix.",
     "Steinbach Werkzeuge GmbH supplies the tooling.",
     [("ORGANIZATION", "Steinbach Werkzeuge GmbH")]),
    ("pos-ner-org-foundation", "A foundation, which is an organisation.",
     "The Ravenswood Foundation donated supplies.",
     [("ORGANIZATION", "Ravenswood Foundation")]),
    ("pos-ner-org-plural", "A plural organisation name.",
     "Two law firms merged to form Harrington & Vale.",
     [("ORGANIZATION", "Harrington & Vale")]),
    ("pos-ner-org-committee", "An organisation named as a committee.",
     "The Petersfield Safety Committee approved the plan.",
     [("ORGANIZATION", "Petersfield Safety Committee")]),
    ("pos-ner-org-university", "A university as an organisation.",
     "Northgate University admitted a larger cohort.",
     [("ORGANIZATION", "Northgate University")]),
    ("pos-ner-org-with-person", "A person and their organisation in one sentence.",
     "Aisha Rahman leads Northgate Ventures.",
     [("PERSON", "Aisha Rahman"), ("ORGANIZATION", "Northgate Ventures")]),
    ("pos-ner-org-mixed", "All three entity types in one sentence.",
     "Ines Alvarado of Cedarline Systems presented the roadmap in Helsinki.",
     [("PERSON", "Ines Alvarado"), ("ORGANIZATION", "Cedarline Systems"), ("LOCATION", "Helsinki")]),

    ("pos-ner-loc-city", "A bare city name.",
     "The conference was held in Lisbon.",
     [("LOCATION", "Lisbon")]),
    ("pos-ner-loc-country", "A country name.",
     "Delegates arrived from Portugal.",
     [("LOCATION", "Portugal")]),
    ("pos-ner-loc-region", "A region or state name.",
     "The office sits in Bavaria.",
     [("LOCATION", "Bavaria")]),
    ("pos-ner-loc-two", "Two place names in one sentence, in text order.",
     "The team flew from Toronto to Zurich.",
     [("LOCATION", "Toronto"), ("LOCATION", "Zurich")]),
    ("pos-ner-loc-institution", "A place name inside an institution name.",
     "The Portland campus opened its new hall.",
     [("LOCATION", "Portland")]),
    ("pos-ner-loc-preposition", "A place name after a preposition.",
     "We shipped from Dunedin to Napier last week.",
     [("LOCATION", "Dunedin"), ("LOCATION", "Napier")]),
    ("pos-ner-loc-city-country", "A city and a country in one sentence.",
     "The treaty was signed in Vienna by Austria.",
     [("LOCATION", "Vienna"), ("LOCATION", "Austria")]),

    ("pos-ner-mixed-two-sent", "Two sentences, each with a person and a place.",
     "Rosa Lindqvist led the review. She then travelled to Madrid.",
     [("PERSON", "Rosa Lindqvist"), ("LOCATION", "Madrid")]),
    ("pos-ner-mixed-org-loc", "An organisation and a place in one sentence.",
     "Sablefish Robotics is based in Auckland.",
     [("ORGANIZATION", "Sablefish Robotics"), ("LOCATION", "Auckland")]),
    ("pos-ner-mixed-three", "Person, organisation and place, all different types.",
     "Tomas Bielik met Lantern Point Press in Krakow.",
     [("PERSON", "Tomas Bielik"), ("ORGANIZATION", "Lantern Point Press"), ("LOCATION", "Krakow")]),

    ("neg-ner-plain-sentence", "A sentence with no name, organisation or place.",
     "The quarterly report was filed on time.", []),
    ("neg-ner-weekday", "A capitalised weekday that is not an entity.",
     "The meeting is scheduled for Monday afternoon.", []),
    ("neg-ner-months", "Capitalised month names that are not entities.",
     "The invoice was issued in January and settled in March.", []),
    ("neg-ner-counts", "Numbers with no name, organisation or place.",
     "There were 42 attendees and 17 proposals submitted.", []),
    ("neg-ner-version", "A capitalised product version that is not an entity.",
     "The team upgraded to Version 3 of the platform.", []),
    ("neg-ner-lower-prose", "Ordinary prose in lower case.",
     "the build server was restarted after the outage", []),
    ("neg-ner-pronouns", "Pronouns where a name would be.",
     "She said they would call him back on Friday.", []),
    ("neg-ner-abstract", "Abstract nouns that are not organisations.",
     "Quality, safety and cost were the deciding factors.", []),
    ("neg-ner-unit", "A measurement with a unit.",
     "The tank holds 500 litres at a pressure of 3 bar.", []),
    ("neg-ner-code", "An identifier that is not a name.",
     "The build identifier is F4X-9921.", []),
    ("neg-ner-url-word", "A sentence about a web address with no entity.",
     "The link is on the first page of the portal.", []),
    ("neg-ner-finance", "A sentence about money with no name or place.",
     "The invoice total was 4200 euros.", []),
    ("neg-ner-time", "A time expression with no entity.",
     "The call starts at 14:30 and should run for an hour.", []),
    ("neg-ner-empty-org-word", "The word 'company' on its own is not an entity.",
     "Every company must file a return.", []),
    ("neg-ner-generic-dept", "A generic department word is not an organisation.",
     "The request went to the finance department.", []),
    ("neg-ner-ordinal", "An ordinal and a quantity.",
     "The third of the five shipments arrived late.", []),
    ("neg-ner-chemical", "A chemical name is not an organisation.",
     "The solvent was mixed with sodium chloride.", []),
    ("neg-ner-language", "A language name is not a location.",
     "The contract is written in Swedish and English.", []),
    ("neg-ner-acronym-only", "A bare acronym with no expansion.",
     "The report cites several standards and frameworks.", []),
    ("neg-ner-month-year", "A month and a year, with no entity.",
     "The archive closes every June and reopens in October.", []),
]

HOLDOUT = [
    ("ho-person-sentence", "Full name as subject of a new sentence.",
     "Corinne Whitlock submitted the revised budget.",
     [("PERSON", "Corinne Whitlock")]),
    ("ho-person-after-hello", "A greeting with a name.",
     "Hello Fatima, your request has been approved.",
     [("PERSON", "Fatima")]),
    ("ho-person-female-surname", "A name whose surname is shared with a city.",
     "Marion Santiago spoke at the conference.",
     [("PERSON", "Marion Santiago")]),
    ("ho-person-three-word", "A three-word name.",
     "Ana Lucia Ferreira Cardoso signed the form.",
     [("PERSON", "Ana Lucia Ferreira Cardoso")]),
    ("ho-person-in-quotes", "A quoted speaker.",
     "The director, Helena Vogt, refused to comment.",
     [("PERSON", "Helena Vogt")]),

    ("ho-org-plc", "A company with a Plc suffix.",
     "Ashcroft Holdings Plc issued a statement.",
     [("ORGANIZATION", "Ashcroft Holdings Plc")]),
    ("ho-org-ab", "A Scandinavian company with an Ab suffix.",
     "Sundby Marine AB won the tender.",
     [("ORGANIZATION", "Sundby Marine AB")]),
    ("ho-org-hospital", "An institution that is an organisation.",
     "Riverside Hospital issued the discharge summary.",
     [("ORGANIZATION", "Riverside Hospital")]),
    ("ho-org-trust", "A trust, which is an organisation.",
     "The Marchmont Trust manages the endowment.",
     [("ORGANIZATION", "Marchmont Trust")]),
    ("ho-org-in-two-sent", "An organisation named in the second sentence.",
     "The audit is complete. Greymoor Associates Ltd approved the accounts.",
     [("ORGANIZATION", "Greymoor Associates Ltd")]),

    ("ho-loc-capital", "A capital city.",
     "Delegations met in Ottawa.",
     [("LOCATION", "Ottawa")]),
    ("ho-loc-river", "A river name.",
     "The bridge over the Danube was reopened.",
     [("LOCATION", "Danube")]),
    ("ho-loc-island", "An island name.",
     "The ferry serves the island of Corsica.",
     [("LOCATION", "Corsica")]),
    ("ho-loc-street", "A street name in an address. It is a PLACE, not an organisation.",
     "The office is at 14 Elm Street.",
     [("LOCATION", "Elm Street")]),
    ("ho-loc-two-countries", "Two country names.",
     "The shipment moved from Norway to Kenya.",
     [("LOCATION", "Norway"), ("LOCATION", "Kenya")]),

    ("ho-neg-weekday", "A weekday, which is not an entity.",
     "We will reconvene on Thursday.", []),
    ("ho-neg-season", "A season name, which is not an entity.",
     "The audit runs in Autumn each year.", []),
    ("ho-neg-dept", "A department, which is not an organisation here.",
     "The budget came from the marketing department.", []),
    ("ho-neg-role", "A job title, which is not a person.",
     "The chief financial officer approved it.", []),
    ("ho-neg-numbers", "A number with a unit, which is not an entity.",
     "The shipment weighs 250 kilograms.", []),
    ("ho-neg-file", "A filename, which is not an entity.",
     "The export is saved as report_final_v2.csv.", []),
    ("ho-neg-error", "An error message, which is not an entity.",
     "The request failed with status code 503.", []),
    ("ho-neg-lower-names", "Lower-case words that look like names but are not.",
     "the total cost of the second order was high", []),
    ("ho-neg-abbrev", "An abbreviation, which is not an entity.",
     "See appendix B for the methodology.", []),
    ("ho-neg-tz", "A timezone label, which is not an entity.",
     "The meeting is scheduled for 09:00 UTC.", []),
]

FINAL = [
    ("f-person-two", "Two names in a sentence.",
     "Tobias Lindgren and Rania Aziz signed the agreement.",
     [("PERSON", "Tobias Lindgren"), ("PERSON", "Rania Aziz")]),
    ("f-person-title", "A name after a courtesy title.",
     "Mr. Aled Morgan attended the meeting.",
     [("PERSON", "Aled Morgan")]),
    ("f-org-co", "A company with a Co suffix.",
     "Wexford Trading Co was founded in 1994.",
     [("ORGANIZATION", "Wexford Trading Co")]),
    ("f-org-llc", "A company with an LLC suffix.",
     "Bright Harbor LLC filed the paperwork.",
     [("ORGANIZATION", "Bright Harbor LLC")]),
    ("f-org-council", "An institutional body.",
     "The Kirkwall Council voted to proceed.",
     [("ORGANIZATION", "Kirkwall Council")]),
    ("f-loc-sea", "A sea or ocean name.",
     "The ferry crosses the Baltic Sea.",
     [("LOCATION", "Baltic Sea")]),
    ("f-loc-three", "Three place names in one sentence.",
     "Flights from Madrid to Lima and then Quito.",
     [("LOCATION", "Madrid"), ("LOCATION", "Lima"), ("LOCATION", "Quito")]),
    ("f-loc-avenue", "A street address. The street NAME is a location; the number is not part of the name.",
     "Ship to 88 Chestnut Avenue.",
     [("LOCATION", "Chestnut Avenue")]),
    ("f-mixed-all", "All three types in one sentence.",
     "Nkechi Obi of Adeyemi Foods spoke in Cape Town.",
     [("PERSON", "Nkechi Obi"), ("ORGANIZATION", "Adeyemi Foods"), ("LOCATION", "Cape Town")]),
    ("f-neg-april", "A month in a date, which is not an entity.",
     "The lease ends on 30 April.", []),
    ("f-neg-german", "A language, which is not an entity.",
     "The manual is available in German and Italian.", []),
    ("f-neg-winter", "A season, which is not not an entity.",
     "The plant closed for the Winter.", []),
    ("f-neg-hours", "A clock time, which is not an entity.",
     "The gate closes at 22:15 each night.", []),
    ("f-neg-percentage", "A percentage, which is not an entity.",
     "The margin improved by 4 percent.", []),
    ("f-neg-initial", "An initial in a name-like shape.",
     "Item A was filed under section B.", []),
    ("f-neg-product", "A brand name reported as a place. This is a KNOWN false positive of the prose backend.",
     "The Orion laptop was recalled.", []),
    ("f-neg-plural", "A plural common noun, which is not an entity.",
     "The reports and invoices were archived.", []),
    ("f-neg-facility", "A generic facility word, which is not an entity.",
     "Every office has a fire exit.", []),
]

# The STRUCTURED-DOCUMENT set.
#
# These cases were written AFTER the two corpora showed the decoder reporting
# JSON keys, HTTP headers and spreadsheet field labels as names. Every one of
# them was a real false positive in production-shaped data, and none of them
# would have been caught by the other three sets.
#
# The two rules they test are:
#   - a span carrying a field-path character is a field, not a name
#   - a span made ENTIRELY of field-label words is a field, not a name
STRUCTURED = [
    ("st-neg-json-key", "A JSON key with a colon.",
     '{"name":"Eli","email":"a@b.example"}', []),
    ("st-neg-json-path", "A JSON path value.",
     '{"path":"/auth/login","method":"POST"}', []),
    ("st-neg-http-header", "An HTTP header name.",
     "Set-Cookie: session=abc123; HttpOnly; Secure", []),
    ("st-neg-samesite", "A cookie attribute.",
     "SameSite=Lax; Path=/; Expires=Mon", []),
    ("st-neg-pipe-table", "A pipe-delimited table header.",
     "Name | Age | City | Status", []),
    ("st-neg-label-value", "A label with a value after a colon.",
     "Reference: 12345 issued on 4 April", []),
    ("st-neg-record-number", "A field label that is all field words.",
     "Record Number 4471", []),
    ("st-neg-account-number", "A field label that is all field words.",
     "Account Number 88213", []),
    ("st-neg-medical-record", "A field label that is all field words.",
     "Medical Record Number 7781", []),
    ("st-neg-street-address", "A field label that is all field words.",
     "Street Address 12 Oak Road", []),
    ("st-neg-customer-id", "A field label that is all field words.",
     "Customer ID 9921", []),
    ("st-neg-employee-id", "A field label that is all field words.",
     "Employee ID 4471", []),
    ("st-neg-social-security", "A field label that is all field words.",
     "Social Security Number 123-45-6789", []),
    ("st-neg-credit-debit", "A field label that is all field words.",
     "Credit/Debit Card Number 4111111111111111", []),
    ("st-neg-ssn-label", "A field label that is all field words.",
     "SSN 123-45-6789", []),
    ("st-neg-iban-label", "A field label that is all field words.",
     "IBAN GB82 WEST 1234 5698 7654 32", []),
    ("st-neg-swift-bic", "A field label that is all field words.",
     "SWIFT BIC DEUTDEFF", []),
    ("st-neg-expiry", "A field label with a structured character.",
     "Expires=Mon 01 Jan 2027", []),
    ("st-neg-facility-form", "A field label that is all field words.",
     "Credit Card Authorization Form 2024", []),
    ("st-neg-vendor-report", "A field label that is all field words.",
     "Vendor Performance Report Q3", []),
    ("st-neg-safety-datasheet", "A field label that is all field words.",
     "Safety Data Sheet Revision 4", []),
    ("st-neg-dob-label", "A field label that is all field words.",
     "Date of Birth 4 April 1990", []),
    ("st-neg-policy-number", "A field label that is all field words.",
     "Policy Number 4471-B", []),
    ("st-neg-claim-id", "A field label that is all field words.",
     "Claim ID 9921-A", []),
    ("st-neg-race-ethnicity", "Demographic field labels, not people.",
     "Race Ethnicity Age 34", []),
    # The real entity must SURVIVE the field-name rules. A name that merely
    # CONTAINS a field word is not a field label.
    ("st-pos-name-in-record", "A real name inside a record with field labels.",
     "Record Number 4471; Name: Ines Alvarado",
     [("PERSON", "Ines Alvarado")]),
    ("st-pos-org-in-record", "A real organisation inside a record with field labels.",
     "Account Number 88213; Company: Halcyon Manufacturing Corp",
     [("ORGANIZATION", "Halcyon Manufacturing Corp")]),
    ("st-pos-loc-in-record", "A real place inside a record with field labels.",
     "City: Copenhagen; Country: Denmark",
     [("LOCATION", "Copenhagen"), ("LOCATION", "Denmark")]),
    ("st-pos-name-with-title", "A real name after a structured label.",
     "Employee ID 4471: Beatriz Nunez signed it.",
     [("PERSON", "Beatriz Nunez")]),
    ("st-pos-accountants", "A name that CONTAINS a field word but is not a field.",
     "Accountants Ltd filed the return.",
     [("ORGANIZATION", "Accountants Ltd")]),

    # These two are the cases that discriminate the field-label rule. The
    # other structured cases are also caught by the structured-character
    # rule, so disabling the label rule alone does not break them; these
    # two are plain words with no punctuation, so only the label rule
    # catches them.
    ("st-neg-two-field-words", "Two field words with no punctuation. Only the field-label rule catches this.",
     "Claim ID 9921-A was filed",
     []),
    ("st-neg-label-value-words", "A label and a value in words. Only the field-label rule catches this.",
     "Vendor Performance Report Q3 was issued",
     []),
]


def build(cases):
    out = []
    for case_id, note, text, entities in cases:
        for typ, value in entities:
            if text.count(value) != 1:
                raise SystemExit(
                    f"case {case_id}: value {value!r} occurs {text.count(value)} times, want 1"
                )
        out.append(
            {
                "id": case_id,
                "note": note,
                "text": text,
                "entities": [{"type": typ, "value": value} for typ, value in entities],
            }
        )
    return out


def main():
    outdir = os.path.join("testdata", "golden", "ner")
    os.makedirs(outdir, exist_ok=True)
    sets = [
        ("ner.json", DEV),
        ("holdout.json", HOLDOUT),
        ("final.json", FINAL),
        ("structured.json", STRUCTURED),
    ]
    for name, cases in sets:
        data = build(cases)
        path = os.path.join(outdir, name)
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(data, fh, indent=2, ensure_ascii=False)
            fh.write("\n")
        print(f"wrote {path}: {len(data)} cases")


if __name__ == "__main__":
    main()
