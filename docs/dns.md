# DNS

Our DNS entries are delegated to zones the maintainers have configured in [Google Cloud DNS](https://cloud.google.com/dns).

We have one zone for scorecard.dev and one for securityscorecards.dev.
The NS values can be found by going to a DNS zone and clicking on "Registrar Setup".

This will look something like:
* ns-cloud-b1.googledomains.com.
* ns-cloud-b2.googledomains.com.
* ns-cloud-b3.googledomains.com.
* ns-cloud-b4.googledomains.com.

This file was imported with the results API from `ossf/scorecard-webapp`, where
it documented DNS for both the website and the API. A path filter cannot split a
file, so it came across whole and the website half was removed here; the website
half stays in `ossf/scorecard-webapp`, which still owns the Netlify deployment
and its certificates.

## API

The API portion is hosted on [Cloud Run](https://cloud.google.com/run).
We map our domain name to the service via Cloud Run domain mapping.
Follow [these instructions](https://cloud.google.com/run/docs/mapping-custom-domains#map) to do the mapping.