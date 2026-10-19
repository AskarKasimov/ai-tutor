# Tutorial 4. Prolog


## Слайд 1

Darko Bozhinoski, Ph.D. in Computer Science

## Слайд 2

## What is Prolog?

Theoretical Foundations

Prolog was developed based on formal logic and mathematical theories of computation.

Declarative Language Pattern Matching

Prolog is a declarative language, which means you describe the problem and let the computer figure out how to solve it.

Prolog uses pattern matching to compare the input with the knowledge base and find solutions to problems.

## Слайд 3

<!-- image -->

## Syntax and Structure

Prolog programs are composed of facts and rules that follow a specific syntax and structure.

## What is Logic Programming?

<!-- image -->

- "The use of mathematical logic for computer programming."(Wikipedia)
- "A declarative, relational style of programming based on first-order logic." (Dictionary.com)

## Logic Programming

<!-- image -->

## Interpreters

Logic programming is based on rules, which consist of a head and a body that define relationships between objects.

Prolog programs are run by an interpreter that matches queries to the knowledge base and produces results.

### Текст изображения 1 (OCR)


```prolog
?- likes(john, jane).
```

dot necessary
true.
answer from prolog interpreter
sign on
prolog query
prompt
variables
7- friends(X, Y).
X = john,
Y = jane ; → type ; to get next solution
X = jane,
Y = john.

### Текст изображения 3 (OCR)

What is Prolog?
Prolog is a logical and a declarative programming
language.
The name itsell, Prolog, is short for PROgramming in
Prolog is a Programming Language for symbolic, non
numeric computation
Prolog is the mejor example of a fourth generation
programming language.

## Слайд 4

## Three Constructs in Prolog

- Facts
- Rules
- Queries

## Слайд 5

## Working with Facts and Rules in Prolog

Facts

Facts are statements that are always true in the knowledge base, which can be used to build further relationships between objects.

Rules

Rules define relationships between objects and can be used to solve problems by querying them with specific input.

## Слайд 6

## KNOWLEDGE BASE

Facts and Rules together comprises a knowledge base of the program.

## Слайд 7

Queries are used on the knowledge base. Syntax: Queries end with a "full stop".

## Слайд 8

<!-- image -->

### Текст изображения 4 (OCR)

Knowledge
Base
Question
main(){
procedure1()
Machine
procedure2()
return
Answer
Logical Programming
Functional Programming

## Слайд 9

## Using Prolog for Logic Programming

<!-- image -->

## Rule-Based Systems

<!-- image -->

<!-- image -->

## Logic Circuits

## Expert Systems

Prolog is great for creating rulebased systems that can be used for problem-solving in various fields.

Prolog can simulate the behavior of digital logic gates and circuits, which can be used for testing logic designs.

Prolog is widely used for creating expert systems that can provide automated advice and decisionmaking.

### Текст изображения 6 (OCR)

NOT
Standerd Logic Gates
Preceding NOT Gate on One limput
for AND; NAND, OR, NOR Gates
NAND
XOR

### Текст изображения 7 (OCR)

SYSTEI
SEMANTIC INTELLIGEI

## Слайд 10

## Disadvantages of Prolog

- Prolog does not have good Input/Output features compared to other languages like LISP.
- Prolog cannot handle statements where one of several things is true, but you don't know which.
- Prolog can be less performant than other languages, particularly for tasks that require a lot of computation.

## Слайд 11

## SWI-Prolog

- SWI-Prolog runs on Windows, Macintosh and L inux platforms.
- Can be used online: https://swish.swi-prolog.org/

## Слайд 12

## DEMO

- [https://swish.swi-prolog.org/](https://swish.swi-prolog.org/)

<!-- image -->

## Слайд 13

## Example

- Lili is happy if she dances.
- Tom is hungry if he is searching for food.
- Andrew and Ilnar are friends if both of them love to play football.
- Ryan will go to play if school is closed, and he is free.

## Слайд 14

## Example

- Lili is happy if she dances.
- Tom is hungry if he is searching for food.
- Andrew and Ilnar are friends if both of them love to play football.
- Ryan will go to play if school is closed, and he is free.

```
Solution:happy(lili) :- dances(lili).
                    hunchy(tom) :- search_for_food(tom).
            friends(Andrew, linear) :- lovesFootball(jack),
            lovesFootball(bili).
            goToPlay(ryan) :- isClosed(school), free(ryan).

```

## Слайд 15

## Data Objects in Prolog

<!-- image -->

Data objects in Prolog

### Текст изображения 9 (OCR)

Simple Objects
Structures
Constants
Varlables
Atoms
Numbers

## Слайд 16

## Arithmetic Expressions

The is built-in predicate is used in Prolog to force the evaluation of arithmetic expressions. If you just write something like X = 2 + 4, the result is to bind X to the unevaluated term 2 + 4, not to 6. Example:

$$\begin{matrix} ? - X = 2 + 4 . \\ X = 2 + 4 \end{matrix}$$

If instead you write X is 2 + 4, Prolog arranges for the second argument, the arithmetic expression 2 + 4, to be evaluated (giving the result 6) before binding the result to X.

$$\begin{matrix} ? - X \, i s \, 2 + 4 . \\ X = 6 \end{matrix}$$

## Слайд 17

| Comparison Definition   |                                                                                        | Evaluates?   |
|-------------------------|----------------------------------------------------------------------------------------|--------------|
| λ = X                   | succeeds if X and Yunify (match) in the Prolog sense                                   | No           |
| X\= ￥                   | succeeds if X and Y do not unify; i.e. if not (X = Y)                                  | No           |
| T1 == T2                | succeeds if terms T1 and T2 are identical; e.g. names of variables have to be the same | No           |
| T1 \== T2               | succeeds if terms T1 and T2 are not identical                                          | No           |
| E1 =:= E2               | succeeds if values of expressions E1 and E2 are equal                                  | Yes          |
| E1 =\= E2               | succeeds if values of expressions E1 and E2 are not equal                              | Yes          |
| E1 < E2                 | succeeds if numeric value of expression E1 is < numeric value of E2                    | Yes          |
| E1 =< E2                | succeeds if numeric value of expression E1 is ≤ numeric value of E2                    | Yes          |
| E1 > E2                 | succeeds if numeric value of expression E1 is > numeric value of E2                    | Yes          |
| E1 >= E2                | succeeds if numeric value of expression E1 is ≥ numeric value of E2                    | Yes          |
| T1 @< T2                | succeeds if T1 is alphabetically < T2                                                  | No           |
| T1 @=< T2               | succeeds if T1 is alphabetically ≤ T2                                                  | No           |
| T1 @> T2                | succeeds if T1 is alphabetically > T2                                                  | No           |
| T1 @>= T2               | succeeds if T1 is alphabetically ≥ T2                                                  | No           |

## Слайд 18

- ❖ Lists:
- [] ➢ [1, 2, 3]
- [4, chan]
- [pen, apple]
- [[pen, pineapple], [apple, pen]]
- ❖ HEADS | TAILS
- [1, 2, 3] % 1 - head, [2, 3] - tail
- [H | T] % "read: H bar T" %
- [H | T] matches with [1, 2, 3] as [1 | [2, 3]]
- ❖ [1, \_, 3] ➢ % could be:
- % [1, 2, 3]
- % [1, x, 3]
- % [1, [......], 3]

## LISTS

## Слайд 19

## Inbuilt predicates in Prolog

- Identify if an element is part of a list:
- member( a, [a, b, c, d]).
- Test for integers in a range:
- maplist(between(1, 3), [1, 2, 3]).

…

[http://www.gprolog.org/manual/html\_node/gprolog024.html](http://www.gprolog.org/manual/html_node/gprolog024.html)

## Слайд 20

## Exercise I

Write a program that compares two numbers.

## Слайд 21

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

## Слайд 22

## Exercise I - Extension

Extend the program to a program that recursively compares each element in the list to find the maximum value.

## Слайд 23

## Exercise I - Extension Solution

Extend the program to a program that recursively compares each element in the list to find the maximum value.

```
max_list([X], X).
    max_list([H|T], Max) :-
        max_list(T, RestMax),
        Max is max(H, RestMax).

```

## Слайд 24

## Exercise II

Write a program that checks if two lists are equal.

## Слайд 25

## Exercise III: Factorial Checker

- Write the program to check if the factorial of the first value is equal to the second value
- Expected outputs:

```prolog
?- factorial(3,6).
```

// true

```prolog
?- factorial(3,5).
```

// false

## Слайд 26

## Exercise IV: Sum of elements

Define a program sum\_list that takes a list of integers as its first argument and returns the sum of all the integers in the list as its second argument.

## Слайд 27

## Exercise V: Palindrome

Define a program palindrome that takes a list as its argument and returns true if the list is a palindrome (i.e., reads the same forwards and backwards).

## Слайд 28

<!-- image -->