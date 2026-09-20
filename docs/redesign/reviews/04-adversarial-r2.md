# Adversarial review of revision 2

Verdict: neither package approvable as written. Most r1 text holes
were closed. Remaining blockers: two schedulers in one spec; `Add`
abort can return nil then drop; `Do` during drain can refill inbound;
budgeted `flow.Run` cannot fill an immutable `Result`; budget clock
unspecified; `Wait` defined only after budgeted return.

Revision 3 is the response.
