# Tutorial 4. Prolog


## Слайд 1

## Exercise I - Solution

Write a program that compares two numbers.

```
Solution 1.
find_max(X, Y, X) :- X >= Y.
find_max(X, Y, Y) :- X < Y.
Solution 2.
gte(X,Y) :- X > Y,write('X is greater').
gte(X,Y) :- X =:= Y,write('X and Y are same').
gte(X,Y) :- X < Y,write('X is smaller').
```