# Week 6 Tutorial_Covered


## Слайд 1

## Travelling Salesman Problem - Simulated Annealing (Pseudocode)

```
- SIMULATED- ANNALING (source,destination, schedule, num_iterations) return a route
          current <- random route from source to destination
          for num_iterations do
          T <- schedule()
          /neighbours <- current.NEIGHBOURS
          next/<- randomly choose one state from neighbours
          eA/<- next.COST - current.COST
          jF eA < 0 then current <- next
          /else current <- next only with probability of e^E/T
          /route <- current
          return route

```

```
return on route
```