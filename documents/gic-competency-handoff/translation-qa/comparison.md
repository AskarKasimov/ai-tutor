# Оригинал / предоставленный перевод / модуль Ваче


Предоставленный перевод — сравнительный черновик, не безошибочный эталон.



## AI-01-02 — Сильный и слабый искусственный интеллект


### Оригинал


[D27 | week1 lecture.pdf | с.27 | core]
Strong & Weak AI
- Strong AI
- Human-based design
- Human-based constructions
- Weak AI
- Problem Solving for specific tasks
- May or may not have human aspects
- Majority of AI work is on this issue


### Предоставленный перевод


[D27 | week1 lecture.pdf | с.27 | core]
Сильный и слабый искусственный интеллект
- Сильный ИИ:
  - проектирование по образцу человека;
  - создание систем, устроенных по человеческому образцу.
- Слабый ИИ:
  - решение конкретных задач;
  - система может воспроизводить отдельные человеческие черты, а может и не воспроизводить;
  - большинство работ в области ИИ посвящено именно этому направлению.


### Модуль Ваче


[D27 | week1 lecture.pdf | с.27 | ядро]
Сильный и слабый ИИ
- Сильный ИИ
- Дизайн, основанный на человеке
- Конструкции, основанные на человеке
- Слабый ИИ
- Решение задач для конкретных целей
- Может иметь или не иметь человеческих аспектов
- Большая часть работы в области ИИ посвящена этой проблеме


## AI-04-07 — Допустимость эвристики


### Оригинал


[D03 | Lecture 5.pdf | с.16 | core]
Heuristics Admissibility
- In path-finding problems heuristics may be admissible or inadmissible
- Admissibility means that heuristics is not overestimating the solution
- Needs to be lower than or equal to the actual cost of reaching the goal
- An admissible heuristic is therefore optimistic
- Not each heuristic is obliged to be admissible. However, inadmissible
heuristic does not guarantee the optimal solution

[D22 | Week 6 Lecture.pdf | с.16 | shared]
Heuristics Admissibility and Consistency
- In path-finding problems heuristics may be admissible or inadmissible
- Admissibility means that heuristics is not overestimating the solution
- Needs to be lower than or equal to the actual cost of reaching the goal
- Not each heuristic is obliged to be admissible. However, inadmissible heuristic
does not guarantee the optimal solution
- In path-finding problems a heuristic is considered to be consistent (or
monotone) if the estimated cost from one node to the successor node, added
to the estimated cost from the successor node to the goal is less than or equal
to the estimated cost from the current node to the goal state
- 𝐸𝑓(𝑥! →𝑥!"#) + 𝐸𝑓(𝑥!"# →𝑥$%&') ≤𝐸𝑓(𝑥! →𝑥$%&') →consistent heuristics
- All consistent heuristics are admissible heuristics, however, all admissible
heuristics are not necessarily consistent heuristics


### Предоставленный перевод


[D03 | Lecture 5.pdf | с.16 | core]
Допустимость эвристики
- В задачах поиска пути эвристика может быть допустимой или недопустимой.
- Допустимость означает, что эвристика не переоценивает истинную стоимость пути до цели.
- Оценка h должна быть не больше фактической минимальной стоимости достижения цели.
- Допустимая эвристика, следовательно, оптимистична.
- Не каждая эвристика обязана быть допустимой. Однако при использовании недопустимой эвристики оптимальность найденного решения не гарантируется.

[D22 | Week 6 Lecture.pdf | с.16 | shared]
Допустимость и согласованность эвристик
- Эвристика допустима, если не переоценивает минимальную стоимость пути до цели.
- Не всякая эвристика обязана быть допустимой, но недопустимая не обеспечивает гарантию оптимального решения.
- Согласованной (монотонной) называется эвристика, для которой оценка стоимости от текущей вершины до цели не больше стоимости перехода к соседу плюс эвристическая оценка от этого соседа до цели.
- Стандартная запись: h(x) ≤ c(x, y) + h(y) для соседних x, y. [Формула на слайде распознана с искажениями; математическое условие восстановлено по определению.]
- Каждая согласованная эвристика допустима, если h(цель) = 0; обратное в общем случае неверно.


### Модуль Ваче


[D03 | Lecture 5.pdf | с.16 | core]
## Допустимость эвристик
- В задачах поиска пути эвристики могут быть допустимыми или недопустимыми
- Допустимость означает, что эвристика не переоценивает решение
- Должна быть меньше или равна фактической стоимости достижения цели
- Поэтому допустимая эвристика является оптимистичной
- Не каждая эвристика обязана быть допустимой. Однако недопустимая эвристика не гарантирует оптимальное решение

[D22 | Week 6 Lecture.pdf | с.16 | shared]
## Допустимость и согласованность эвристик
- В задачах поиска пути эвристики могут быть допустимыми или недопустимыми
- Допустимость означает, что эвристика не переоценивает решение
- Должна быть меньше или равна фактической стоимости достижения цели
- Не каждая эвристика обязана быть допустимой. Однако недопустимая эвристика не гарантирует оптимальное решение
- В задачах поиска пути эвристика считается согласованной (или монотонной), если оценочная стоимость от одного узла до последующего узла, прибавленная к оценочной стоимости от последующего узла до цели, не превышает оценочную стоимость от текущего узла до целевого состояния
- 𝐸𝑓(𝑥! →𝑥!"#) + 𝐸𝑓(𝑥!"# →𝑥$%&') ≤𝐸𝑓(𝑥! →𝑥$%&') → согласованные эвристики
- Все согласованные эвристики являются допустимыми, однако не все допустимые эвристики обязательно согласованы


## AI-04-23 — Измерение сложности ландшафта случайными блужданиями


### Оригинал


[D25 | Week 7 Lecture.pdf | с.52 | core]
Quantifying the Space
Take a random walk about the space – we are not interested in the
optimization but knowing how the space looks to the operators
Take the connections mixed with the fitness values, who is a neighbor of
who
Use this measure to get an idea of how the space looks – surveying the
space

[D25 | Week 7 Lecture.pdf | с.53 | core]
Information Content
Uses principles from Information theory to give a measure of the ruggedness
of the space
Transform the space or area in a space into a single quantifiable value
based on the Entropy
Idea is to make random walks over the space, score them based on the
gradient, then check the amount of variation in these changes

[D25 | Week 7 Lecture.pdf | с.54 | core]
Three Modalities
We move from higher to lower fitness beyond a tolerance
Score as a -1
We keep the same fitness within a tolerance
Score as a 0
We move from a lower to higher fitness beyond a tolerance
Score as a 1
A smooth space will have a very little change in these values
(0,0,1,1,1,1,1,1,0)
A rugged space will have a ‘randomness’ in the values (0,1,-1,-1,0,1,1,0)

[D25 | Week 7 Lecture.pdf | с.55 | core]
Information Content
Take a number of random walks from p to gq, compute the entropy
For a random walk from p to q
Pr[pq] — the proportions found of each of the fitness movements on the walks
Log base 6
10


### Предоставленный перевод


[D25 | Week 7 Lecture.pdf | с.52 | core]
Количественное описание пространства поиска
- Выполните случайное блуждание по пространству: цель — не оптимизировать решение, а выяснить, каким пространство выглядит для выбранных операторов поиска.
- Учитывайте связи между соседними состояниями и соответствующие значения приспособленности (fitness): кто чей сосед.
- Полученную меру используют для исследования структуры пространства.

[D25 | Week 7 Lecture.pdf | с.53 | core]
Информационное содержание
- На основе теории информации можно измерять неровность (ruggedness) ландшафта поиска.
- Область пространства описывают одним числом, вычисляемым с использованием энтропии.
- Идея: выполнить случайные блуждания, оценить изменения fitness вдоль пути и определить разнообразие таких изменений.

[D25 | Week 7 Lecture.pdf | с.54 | core]
Три типа изменений
- Переход к существенно меньшему fitness (ниже порога допуска): −1.
- Fitness остаётся тем же в пределах допуска: 0.
- Переход к существенно большему fitness: +1.
- Для гладкого ландшафта последовательность изменений относительно однородна: (0, 0, 1, 1, 1, 1, 1, 1, 0).
- Для неровного ландшафта характерны нерегулярные колебания: (0, 1, −1, −1, 0, 1, 1, 0).

[D25 | Week 7 Lecture.pdf | с.55 | core]
Информационное содержание
- Возьмите несколько случайных блужданий между обозначенными точками, например p и q, и рассчитайте энтропию.
- Pr[pq] — доля переходов каждого типа, наблюдаемых при блужданиях.
- Используется логарифм по основанию 6.
- На слайде приведён числовой пример: 10.
[Точное расположение обозначений и математическое выражение требуют проверки PDF.]


### Модуль Ваче


Quantifying the Space
Квантование пространства
Take a random walk about the space – we are not interested in the
optimization but knowing how the space looks to the operators
Выполнить случайное блуждание по пространству — нас не интересует оптимизация, а понимание того, как пространство выглядит для операторов
Take the connections mixed with the fitness values, who is a neighbor of
who
Учесть связи, смешанные со значениями приспособленности, кто является соседем кого
Use this measure to get an idea of how the space looks – surveying the
space
Использовать эту меру, чтобы получить представление о том, как выглядит пространство — обследование пространства

Information Content
Содержание информации
Uses principles from Information theory to give a measure of the ruggedness
of the space
Использует принципы теории информации для оценки «рельефности» пространства
Transform the space or area in a space into a single quantifiable value
based on the Entropy
Преобразовать пространство или область в единственное измеримое значение,
основанное на энтропии
Idea is to make random walks over the space, score them based on the
gradient, then check the amount of variation in these changes
Идея состоит в том, чтобы выполнять случайные блуждания по пространству, оценивать их по градиенту, а затем проверять степень вариации этих изменений

Three Modalities
Три модальности
We move from higher to lower fitness beyond a tolerance
Score as a -1
Мы переходим от более высокой к более низкой приспособленности за пределами допуска
Оценка: -1
We keep the same fitness within a tolerance
Score as a 0
Мы сохраняем ту же приспособленность в пределах допуска
Оценка: 0
We move from a lower to higher fitness beyond a tolerance
Score as a 1
Мы переходим от более низкой к более высокой приспособленности за пределами допуска
Оценка: 1
A smooth space will have a very little change in these values
(0,0,1,1,1,1,1,1,0)
Гладкое пространство будет характеризоваться почти отсутствием изменений в этих значениях
(0,0,1,1,1,1,1,1,0)
A rugged space will have a ‘randomness’ in the values (0,1,-1,-1,0,1,1,0)
Рельефное пространство будет проявлять «случайность» в значениях (0,1,-1,-1,0,1,1,0)

Information Content
Содержание информации
Take a number of random walks from p to gq, compute the entropy
Выполнить несколько случайных блужданий от p к gq и вычислить энтропию
For a random walk from p to q
Для случайного блуждания от p к q
Pr[pq] — the proportions found of each of the fitness movements on the walks
Pr[pq] — пропорции, найденные для каждого из перемещений приспособленности в ходе блужданий
Log base 6
Логарифм по основанию 6
10
10


## AI-08-05 — Активации ReLU, Leaky ReLU, PReLU, Softplus


### Оригинал


[D13 | Week 12 Lecture.pdf | с.10 | core]
Activation Functions
- Generally non-linear
- Otherwise output is input
- Required to be differentiable
in backpropagation training
- Uses a calculation of error
- Needs to get the integration of
the differentiation to find error
a= purelin(n)
a = hardlims(n)
Linear Transfer Function
Symmetric Hard Limit Trans. Funct.
0
1
a = tansig(n)
Satlin Transfer Function
Tan-Sigmoid Transfer Function
10
0.833
+0.833
Log-Sigmoid Transfer Function
Radial Basis Function

[D13 | Week 12 Lecture.pdf | с.11 | core]
Rectified Linear Unit (ReLU)
- Where u is an input to neuron
- One of the most popular activation
functions
f(u) = max(0, uw)

[D13 | Week 12 Lecture.pdf | с.12 | core]
Leaky ReLU
- More advanced ReLU
- 𝛼is a hyperparameter chosen and
fixed initially
- Usually, small value
fla)
f(a) = mazx(aa, a)

[D13 | Week 12 Lecture.pdf | с.13 | core]
Parametric ReLU
- Advanced Leaky ReLU
- 𝛼parameter can be trained
= max(aa, a)

[D13 | Week 12 Lecture.pdf | с.14 | core]
Softplus
- Softplus is a
approximation to a
ReLU which is
f(u) = ln(1+eu)
— Rectifier
2
1
2
3


### Предоставленный перевод


[D13 | Week 12 Lecture.pdf | с.10 | core]
Функции активации
- Как правило, нелинейны: без нелинейности композиция слоёв остаётся линейным преобразованием.
- При обучении обратным распространением ошибки обычно требуется возможность вычислять производную функции активации (оригинал формулирует это как требование дифференцируемости).
- Обучение использует вычисление ошибки и её производных.
- Примеры функций на рисунке: линейная (purelin), симметричная пороговая (hardlims), насыщаемая линейная (satlin), гиперболический тангенс (tansig), логистическая сигмоида, радиальная базисная функция.
[Графики функций приведены на исходном слайде.]

[D13 | Week 12 Lecture.pdf | с.11 | core]
Выпрямленный линейный блок (ReLU)
- u — вход нейрона.
- Одна из самых популярных функций активации.
- Формула на слайде распознана как f(u) = max(0, uw); предположительно имеется в виду f(u) = max(0, u), но нужно сверить PDF.

[D13 | Week 12 Lecture.pdf | с.12 | core]
Leaky ReLU
- Модификация ReLU, пропускающая небольшой отрицательный сигнал.
- α — гиперпараметр, выбираемый и фиксируемый до обучения; обычно он мал.
- Формула повреждена при извлечении: fla); f(a) = mazx(aa, a).

[D13 | Week 12 Lecture.pdf | с.13 | core]
Параметрическая ReLU (PReLU)
- Развитие Leaky ReLU.
- Параметр α допускается обучать.
- Формула на слайде распознана с ошибками: = max(aa, a).

[D13 | Week 12 Lecture.pdf | с.14 | core]
Softplus
- Гладкая аппроксимация ReLU.
- Формула: f(u) = ln(1 + eᵘ).
[Графики и фрагменты формул на слайдах необходимо сверить с исходными PDF.]


### Модуль Ваче


Activation Functions
- Обычно нелинейные
- Иначе выход равен входу
- Требуется дифференцируемость при обучении методом обратного распространения
- Использует вычисление ошибки
- Необходимо выполнить интегрирование дифференцирования для нахождения ошибки

a = purelin(n)
a = hardlims(n)

**Linear Transfer Function**
**Symmetric Hard Limit Transfer Function**
0
1

a = tansig(n)
**Satlin Transfer Function**
**Tan‑Sigmoid Transfer Function**
10
0.833
+0.833
**Log‑Sigmoid Transfer Function**
**Radial Basis Function**

---

## Слайд 11
**Rectified Linear Unit (ReLU)**
- где $u$ — вход нейрона
- одна из самых популярных функций активации
- $f(u) = \max(0, uw)$

---

## Слайд 12
**Leaky ReLU**
- более продвинутый ReLU
- $\alpha$ — гиперпараметр, выбранный и фиксированный изначально
- обычно небольшое значение
- $f(a) = \max(aa, a)$

---

## Слайд 13
**Parametric ReLU**
- продвинутый Leaky ReLU
- параметр $\alpha$ может обучаться
- $= \max(aa, a)$

---

## Слайд 14
**Softplus**
- Softplus — приближение к ReLU, которое
- $f(u) = \ln(1+e^{u})$
- — Выпрямитель

2
1
2
3


## AI-10-01 — Генеративные и дискриминативные модели


### Оригинал


[D01 | 14 Tutorial.pdf | с.3 | core]
What is Generative AI?
- AI systems that generate new content: text, images, code,
audio, video
- Trained on huge datasets; learns patterns and
probabilities
- Generates outputs that were not explicitly seen during
training

[D01 | 14 Tutorial.pdf | с.4 | core]
Classic AI vs Generative AI
- Classic AI: rules, logic, search, optimization; outputs decisions/labels
- Examples: path planning, chess engines, spam filters
- Generative AI: predicts next token to create new content
- Examples: ChatGPT answers, code completion, story generators
- They are complementary, not competing approaches

[D16 | Week 14 Lecture Slides.pdf | с.5 | core]
- Machine Learning (ML) is a field of study in AI
concerned with the development and study
of statistical algorithms that can learn from
data and generalise to unseen data, and
thus perform tasks without explicit instructions
- Deep Learning (DL) is a part of ML
- DL focuses on utilizing multilayered neural
networks to perform tasks such as
classification, regression, and representation
learning
- ML uses both discriminative models and
generative models to predict data
Machine Learning
Discriminative Neural Network
0.03 Bird
0.05
Dog
Output
Inpu
Generative Neural Network
“photo of a
tabby cat"
tab.
cat
Input
Output


### Предоставленный перевод


[D01 | 14 Tutorial.pdf | с.3 | core]
Что такое генеративный ИИ?
- Системы ИИ, создающие новое содержимое: текст, изображения, код, звук, видео.
- Обучаются на больших наборах данных, выявляя закономерности и вероятностные зависимости.
- Создают результаты, которые не встречались в обучающих данных в точно таком же виде.

[D01 | 14 Tutorial.pdf | с.4 | core]
Классический ИИ и генеративный ИИ
- Классический ИИ: правила, логика, поиск, оптимизация; результатом обычно служит решение или метка.
- Примеры: планирование маршрута, шахматные программы, фильтры спама.
- Генеративный ИИ: например, предсказывает следующий токен, создавая новое содержимое.
- Примеры: ответы ChatGPT, автодополнение программного кода, генераторы историй.
- Эти подходы дополняют друг друга, а не конкурируют.

[D16 | Week 14 Lecture Slides.pdf | с.5 | core]
- Машинное обучение (ML) — область ИИ, посвящённая разработке и исследованию статистических алгоритмов, способных учиться по данным и обобщать знания на новые данные, выполняя задачи без явного программирования всех инструкций.
- Глубокое обучение (DL) — часть машинного обучения.
- В DL для классификации, регрессии и обучения представлениям применяются многослойные нейронные сети.
- В машинном обучении используются как дискриминативные, так и генеративные модели.
Подписи схемы: машинное обучение; дискриминативная нейронная сеть (классы «птица» и «собака» с оценками 0,03 и 0,05); генеративная нейронная сеть (промпт «фото полосатого кота»); вход и выход.
[Расположение объектов на рисунке нужно проверить по PDF.]


### Модуль Ваче


[D01 | 14 Tutorial.pdf | с.3 | core]
Что такое генеративный ИИ?
- Системы ИИ, генерирующие новый контент: текст, изображения, код, аудио, видео
- Обучаются на огромных наборах данных; изучают паттерны и вероятности
- Генерируют выводы, которые не встречались явно во время обучения

[D01 | 14 Tutorial.pdf | с.4 | core]
Классический ИИ vs генеративный ИИ
- Классический ИИ: правила, логика, поиск, оптимизация; выводит решения/метки
- Примеры: планирование пути, шахматные движки, спам‑фильтры
- Генеративный ИИ: предсказывает следующий токен для создания нового контента
- Примеры: ответы ChatGPT, автодополнение кода, генераторы рассказов
- Они дополняют друг друга, а не являются конкурирующими подходами

[D16 | Week 14 Lecture Slides.pdf | с.5 | core]
- Машинное обучение (ML) — область исследования в ИИ, занимающаяся разработкой и изучением статистических алгоритмов, способных обучаться на данных и обобщать их на невидимые данные, тем самым выполняя задачи без явных инструкций
- Глубокое обучение (DL) является частью ML
- DL фокусируется на использовании многослойных нейронных сетей для выполнения задач таких как классификация, регрессия и обучение представлениям
- ML использует как дискриминативные модели, так и генеративные модели для предсказания данных

Machine Learning
Discriminative Neural Network
0.03 Bird
0.05 Dog
Output
Input
Generative Neural Network
“photo of a tabby cat"
tab. cat
Input
Output
